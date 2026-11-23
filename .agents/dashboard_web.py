#!/usr/bin/env python3
"""Live, n8n-style map of the Athena beta: tasks, workers, the master, progress and errors.

Run: python3 .agents/dashboard_web.py   then open http://127.0.0.1:8765
The page polls /state every 2 seconds and /detail?id=X when a node is clicked.
Sources, read fresh on each poll: .kiro/specs/athena-beta/tasks.md (tasks, briefs,
checked = merged), `hc ls` and `hc read` (master, workers, their screens),
git (worktrees, branches merged or not into main, commits ahead, uncommitted files).
Dependencies are not data in tasks.md; they live in EDGES below.
The page itself is .agents/dashboard.html, re-read on every load.
"""
import glob, json, os, re, shutil, subprocess, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
TASKS = os.path.join(ROOT, ".kiro/specs/athena-beta/tasks.md")
PAGE = os.path.join(HERE, "dashboard.html")
STY = os.path.join(HERE, "stylus")
ICONS = os.path.expanduser("~/.config/herdr-app/icons")  # Herdr's bundled agent logos (LobeHub, MIT)
PORT = 8765
SECTIONS = {"A": "Foundation", "B": "Platform", "C": "Kernel", "D": "Product", "E": "Beta release"}
EDGES = [  # design.md section 6, plus the B-group order in tasks.md
    ("B1", "B2"), ("B2", "B3"), ("B3", "B3a"), ("B3", "B4"), ("B3", "B12"), ("B2", "B6"), ("B3", "B9a"),
    ("B2", "B5"), ("B2", "B8"), ("B2", "B10"), ("B2", "B7"), ("B3", "B2a"),
    ("B4", "B4a"), ("B4", "B4b"), ("B7", "B4b"), ("B9a", "B9b"), ("B9b", "B9"), ("B5", "B11"),
    ("C1", "C5"), ("C5", "C6"), ("C6", "C9"), ("C2", "C6"), ("C2", "C10"), ("B9", "C10"),
    ("C3", "D5"), ("D5", "D6"), ("D6", "D7"), ("C4", "C8"), ("C7", "C8"), ("C4", "D4"), ("C7", "D4"),
    ("C3", "D3"), ("B5", "D3"), ("B11", "D5"), ("B2", "D1"), ("B2", "D2"),
    ("C9", "E1"), ("C10", "E1"), ("D7", "E1"), ("D3", "E1"), ("E1", "E2"), ("E2", "E3"),
]
ERR = re.compile(r"(--- FAIL|^FAIL\b|\bBLOCKED\b|panic:|\bError:|\berror:|Traceback|cannot save terminals|ERR_|exit status [1-9])")


# Read-only git: never take .git/index.lock (a background `git status` refreshing
# the index makes the master's commits fail with "index.lock exists").
GIT_ENV = {**os.environ, "GIT_OPTIONAL_LOCKS": "0"}


def run(args, cwd=ROOT, timeout=6):
    try:
        return subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=timeout, env=GIT_ENV).stdout
    except Exception:
        return ""


def clean(s):
    s = re.sub(r"\*\*|`", "", s)
    s = re.sub(r"\[([^\]]+)\]\([^)]+\)", r"\1", s)
    return re.sub(r"\s+", " ", s).strip()


def read_tasks():
    out, section, cur = [], "", None
    try:
        lines = open(TASKS, encoding="utf-8").read().splitlines()
    except OSError:
        return out
    for ln in lines:
        if ln.startswith("## "):
            m = re.match(r"## ([A-E])\.", ln)
            section, cur = (m.group(1) if m else ""), None
            continue
        m = re.match(r"- \[( |x)\] \*\*([A-Z]+\d+[a-z]?)\. ([^*]+?)\*\*\s*(.*)", ln)
        if m and section:
            rest = m.group(4)
            owner = re.match(r"\(([^)]*)\)", rest)
            cur = {"id": m.group(2), "title": m.group(3).rstrip(". "), "sec": section, "done": m.group(1) == "x",
                   "owner": clean(owner.group(1)) if owner else "",
                   "summary": clean(re.sub(r"^\([^)]*\)\s*[:—-]?\s*", "", rest)), "points": []}
            out.append(cur)
            continue
        if cur and re.match(r"\s+- ", ln):
            cur["points"].append(clean(ln.strip()[2:]))
        elif cur and ln.strip() and not ln.startswith(" "):
            cur = None
    for t in out:
        pts = [p for p in t["points"] if not p.lower().startswith(("accept", "carry-over"))]
        about = t["summary"] if len(t["summary"]) > 25 and not t["summary"].startswith("R") else (pts[0] if pts else t["summary"])
        about = re.sub(r"^(Server|Desk|Migration):\s*", "", about)
        if len(about) < 12:
            about = (t["owner"][:1].upper() + t["owner"][1:] + ". " if t["owner"].lower().startswith("needs") else "") + t["title"]
        t["about"] = about if len(about) <= 150 else about[:147].rsplit(" ", 1)[0] + "…"
        t["accept"] = [p[len("Accept: "):] if p.startswith("Accept: ") else p for p in t["points"] if p.lower().startswith("accept")]
        # "Slice 2 merged", "Slice 1 (server: ...) merged", "Slice 2.1 merged": one count per slice id
        t["slices"] = len({m.group(1) for p in t["points"]
                           for m in re.finditer(r"\bSlice\s+(\d+(?:\.\d+)?[a-z]?)(?:\s*\([^)]*\))?\s+merged", p)})
    return out


def worktrees():
    res, cur = [], {}
    for ln in run(["git", "worktree", "list", "--porcelain"]).splitlines() + [""]:
        if not ln:
            if cur.get("worktree"):
                res.append(cur)
            cur = {}
        elif " " in ln:
            k, v = ln.split(" ", 1)
            cur[k] = v
    for w in res:
        b = w.get("branch", "").replace("refs/heads/", "")
        w["branch"] = b
        n = run(["git", "rev-list", "--count", f"main..{b}"]).strip() if b and b != "main" else "0"
        w["ahead"] = int(n) if n.isdigit() else 0
        st = run(["git", "status", "--porcelain"], cwd=w["worktree"]).splitlines()
        w["dirty"] = len([l for l in st if not l.startswith("??")])
        w["untracked"] = len([l for l in st if l.startswith("??")])
        w["last"] = run(["git", "log", "-1", "--format=%h %s (%cr)", b or "HEAD"]).strip()
        w["key"] = task_key(w["worktree"])
    return res


def task_key(path):
    m = re.match(r"athena-erp-([a-z0-9]+)", os.path.basename(path.rstrip("/")))
    return m.group(1).lower() if m else None


def match(key, tid):
    return bool(key) and (key == tid.lower() or (tid.lower() == "b9" and key == "b9"))


_screens = {}


def screen(addr, max_age=5):
    t, txt = _screens.get(addr, (0, ""))
    if time.time() - t > max_age:
        txt = run(["hc", "read", addr], timeout=4)
        _screens[addr] = (time.time(), txt)
    return txt


ROLE_BY_PREFIX = [("sw-", "subworker"), ("sm-", "submaster"), ("th-", "thinker"), ("w-", "worker"), ("r-", "reviewer")]


def jsonl(name, last=None):
    try:
        with open(os.path.join(STY, name)) as f:
            rows = [json.loads(l) for l in f if l.strip()]
    except (FileNotFoundError, ValueError):
        return []
    return rows[-last:] if last else rows


def role_of(name):
    return next((r for p, r in ROLE_BY_PREFIX if name.startswith(p)), None)


def agents():
    try:
        d = json.loads(run(["hc", "ls", "--all"]) or "{}")
    except ValueError:
        d = {}
    terms = d.get("terminals", [])
    live = [t for t in terms if t.get("state") == "live"]
    stylus = LIMITS().get("stylus", "t6")
    masters = [dict(t, role="master") for t in live if t.get("name") == "master"]
    masters += [dict(t, role="stylus", name="Stylus") for t in live if t["address"] == stylus]
    workers = [dict(t, role=role_of(t.get("name") or "")) for t in live if role_of(t.get("name") or "")]
    recent = (time.time() - 3 * 3600) * 1000
    dead = [t for t in terms if t.get("state") != "live" and t.get("exit_code") not in (None, 0) and (t.get("finished_ms") or 0) > recent]
    names = {t["address"]: ("stylus" if t["address"] == stylus else t.get("name") or t["address"]) for t in terms}
    return masters, workers, dead, names


def LIMITS():
    try:
        return json.load(open(os.path.join(STY, "limits.json")))
    except (FileNotFoundError, ValueError):
        return {}


def lanes():
    try:
        return json.load(open(os.path.join(STY, "lanes.json")))
    except (FileNotFoundError, ValueError):
        return {"lanes": []}


def mem_free():
    out = run(["memory_pressure"])
    m = re.search(r"free percentage:\s*(\d+)", out)
    return int(m.group(1)) if m else None


def safe_ops(wout, tasks):
    """The Operations panel must never take the whole map down."""
    try:
        return ops(wout, tasks)
    except Exception as e:  # a malformed record in a .jsonl file
        return {"error": repr(e), "lanes": [], "comms": [], "progress": [], "suite": {}, "resources": {}}


def ops(wout, tasks):
    lane_cfg = lanes()
    by_id = {t["id"]: t for t in tasks}
    lane_out = []
    for ln in lane_cfg.get("lanes", []):
        ts = [{"id": i, "state": by_id[i]["state"] if i in by_id else "todo", "title": by_id.get(i, {}).get("title", i)}
              for i in ln["tasks"]]
        ags = [w["name"] for w in wout if w.get("lane") == ln["id"] or (w.get("task") and w["task"] in ln["tasks"])]
        lane_out.append({**ln, "tasks": ts, "agents": ags,
                         "current": next((t["id"] for t in ts if t["state"] != "merged"), None)})
    try:
        holder = json.load(open(os.path.join(STY, "locks", "suite", "owner.json")))
    except (FileNotFoundError, ValueError):
        holder = None
    srows = jsonl("suite.jsonl", 200)
    queue = []
    for r in srows:
        k = (r.get("who"), r.get("label"), r.get("cwd"))
        if r.get("event") == "queued":
            queue = [q for q in queue if (q.get("who"), q.get("label"), q.get("cwd")) != k] + [r]
        elif r.get("event") in ("start", "end", "gave-up", "reused"):
            queue = [q for q in queue if (q.get("who"), q.get("label"), q.get("cwd")) != k]
    ps = run(["ps", "-axo", "pid=,command="])
    live_pids = {int(l.split(None, 1)[0]) for l in ps.splitlines() if l.strip()}
    queue = [q for q in queue if (q["pid"] in live_pids if "pid" in q else ("--label " + (q.get("label") or "")) in ps)]
    lim = LIMITS()
    du = shutil.disk_usage("/")
    return {"lanes": lane_out, "blocked": lane_cfg.get("blocked", {}),
            "comms": jsonl("comms.jsonl", 150), "progress": jsonl("progress.jsonl", 30),
            "suite": {"holder": holder, "queue": queue, "recent": [r for r in srows if r.get("event") == "end"][-12:]},
            "resources": {"memFreePct": mem_free(), "diskFreeGB": round(du.free / 1e9, 1),
                          "agents": len(wout), "caps": {k: v for k, v in lim.items() if k.startswith(("max", "min"))}},
            "spawned": jsonl("agents.jsonl", 60)}


def state():
    tasks, wts = read_tasks(), worktrees()
    masters, workers, dead, names = agents()
    spawned = {r["name"]: r for r in jsonl("agents.jsonl") if r.get("event") == "spawn"}
    ids = {t["id"] for t in tasks}
    edges = [(a, b) for a, b in EDGES if a in ids and b in ids]
    errors = []
    wout = []
    for w in workers:
        name = w.get("name") or w["address"]
        rec = spawned.get(name, {})
        m = re.match(r"^(?:w|r|sm|sw)-([a-z0-9]+)", name)
        guess = m.group(1).upper() if m else ""
        task = next((t["id"] for t in tasks if t["id"].upper() == (rec.get("task") or "").upper()), None) \
            or next((t["id"] for t in tasks if t["id"].upper() == guess), None) \
            or next((t["id"] for t in tasks if match(task_key(w.get("cwd", "")), t["id"])), None)
        lines = [l.rstrip() for l in screen(w["address"]).splitlines() if l.strip()]
        tail = [l for l in lines if not re.search(r"^[─━\s]+$|^\s*Skills:|Voice input|^❯|muse-spark|ctrl\+o", l)][-8:]
        for l in lines[-200:]:
            if ERR.search(l) and "0 errors" not in l:
                errors.append({"who": name, "task": task, "text": l.strip()[:220]})
        wout.append({"addr": w["address"], "name": name, "status": w.get("status") or "idle",
                     "turn": w.get("turn", 0), "task": task, "cli": rec.get("cli") or w.get("agent") or "agent",
                     "lane": rec.get("lane", ""), "parent": names.get(w.get("parent"), rec.get("parent", "")),
                     "role": w["role"], "cwd": w.get("cwd", ""), "tail": tail})
    for t in dead[-5:]:
        errors.append({"who": t.get("name") or t["address"], "task": None, "text": f"terminal exited with code {t.get('exit_code')}"})
    busy = {}
    for w in wout:
        if w["task"]:
            busy.setdefault(w["task"], []).append(w)
    out = []
    for t in tasks:
        wt = next((w for w in wts if match(w["key"], t["id"])), None)
        ws = busy.get(t["id"], [])
        w = ws[0] if ws else None
        if t["done"]:
            s = "merged"
        elif any(x["status"] == "working" and x["role"] in ("worker", "submaster", "subworker") for x in ws):
            s = "running"
        elif wt and wt["ahead"]:
            s = "review"
        elif w:
            s = "waiting"
        elif wt:
            s = "started"
        else:
            s = "todo"
        blocked = [a for a, b in edges if b == t["id"] and not next(x for x in tasks if x["id"] == a)["done"]]
        out.append({**{k: t[k] for k in ("id", "title", "sec", "owner", "about", "slices")}, "state": s,
                    "branch": wt["branch"] if wt else "", "ahead": wt["ahead"] if wt else 0,
                    "dirty": wt["dirty"] if wt else 0, "agents": [x["addr"] for x in ws],
                    "blockedBy": blocked if s == "todo" else []})
    main_tip = run(["git", "rev-parse", "main"]).strip()
    busy_br = {w["branch"] for w in wts if w["dirty"]}
    merged_br = [b for b in (x.strip(" *+") for x in run(["git", "branch", "--merged", "main"]).splitlines())
                 if b != "main" and b not in busy_br and run(["git", "rev-parse", b]).strip() != main_tip]
    unmerged_br = sorted({b.strip(" *+") for b in run(["git", "branch", "--no-merged", "main"]).splitlines()} | busy_br)
    secs = []
    for k, name in SECTIONS.items():
        ts = [t for t in out if t["sec"] == k]
        secs.append({"key": k, "name": name, "total": len(ts), "merged": sum(t["state"] == "merged" for t in ts),
                     "active": sum(t["state"] in ("running", "waiting", "review", "started") for t in ts),
                     "slices": sum(t["slices"] for t in ts if t["state"] != "merged")})
    ahead_main = run(["git", "rev-list", "--count", "origin/main..main"]).strip()
    return {"time": time.strftime("%H:%M:%S"), "main": run(["git", "rev-parse", "--short", "main"]).strip(),
            "unpushed": int(ahead_main) if ahead_main.isdigit() else 0,
            "ops": safe_ops(wout, out),
            "master": [{"addr": m["address"], "role": m["role"], "cli": "kiro" if m["role"] == "stylus" else (m.get("agent") or "muse"),
                        "parent": "" if m["role"] == "stylus" else "stylus", "name": m.get("name") or m["address"], "status": m.get("status") or "",
                        "cwd": m.get("cwd", ""), "tail": [l for l in screen(m["address"]).splitlines() if l.strip()][-10:]} for m in masters],
            "workers": wout, "tasks": out, "edges": edges, "sections": secs, "errors": errors[-30:],
            "branches": {"merged": merged_br, "unmerged": unmerged_br},
            "worktrees": [{k: w[k] for k in ("worktree", "branch", "ahead", "dirty", "untracked", "last", "key")} for w in wts]}


def detail(tid):
    t = next((x for x in read_tasks() if x["id"].lower() == tid.lower()), None)
    if not t:
        return {}
    briefs = sorted(glob.glob(os.path.join(HERE, "tasks", f"*{t['id'].lower()}*", "*.md")) +
                    glob.glob(os.path.join(HERE, "tasks", f"HANDOVER*{t['id'].lower()}*.md")))
    files = []
    for p in briefs[:8]:
        try:
            files.append({"path": os.path.relpath(p, ROOT), "head": open(p, encoding="utf-8").read()[:1500]})
        except OSError:
            pass
    wt = next((w for w in worktrees() if match(w["key"], t["id"])), None)
    commits, changed = [], []
    if wt and wt["branch"] != "main":
        commits = run(["git", "log", "--format=%h %s (%cr)", f"main..{wt['branch']}"]).splitlines()[:15]
        changed = run(["git", "status", "--short"], cwd=wt["worktree"]).splitlines()[:40]
    return {**t, "worktree": wt, "commits": commits, "changed": changed, "files": files,
            "needs": [a for a, b in EDGES if b == t["id"]], "unlocks": [b for a, b in EDGES if a == t["id"]]}


STYLUS = os.path.join(HERE, "stylus")


def report():
    def rd(name):
        try:
            return open(os.path.join(STYLUS, name), encoding="utf-8").read()
        except OSError:
            return ""
    try:
        shift = json.loads(rd("shift.json") or "{}")
    except ValueError:
        shift = {}
    base = shift.get("mainAtStart", "")
    commits = run(["git", "log", "--first-parent", "--format=%h %ad %s", "--date=format:%H:%M", f"{base}..main"]).splitlines() if base else []
    before = set()
    if base:
        for m in re.finditer(r"- \[x\] \*\*([A-Z]+\d+[a-z]?)\.", run(["git", "show", f"{base}:.kiro/specs/athena-beta/tasks.md"])):
            before.add(m.group(1))
    tasks = read_tasks()
    done_now = [t["id"] + " " + t["title"] for t in tasks if t["done"] and t["id"] not in before]
    log = rd("night-log.md")
    words = {}
    for m in re.finditer(r"^- \d\d:\d\d ([A-Z][A-Z_]+(?: [A-Z][A-Z_]+)?)", log, re.M):
        words[m.group(1)] = words.get(m.group(1), 0) + 1
    files = run(["git", "diff", "--shortstat", f"{base}..main"]).strip() if base else ""
    return {"shift": shift, "report": rd("night-report.md"), "log": log, "commits": commits, "tasksDone": done_now,
            "counts": words, "diffstat": files, "now": time.strftime("%Y-%m-%d %H:%M")}


class H(BaseHTTPRequestHandler):
    def do_GET(self):
        u = urlparse(self.path)
        try:
            if u.path == "/state":
                body, ctype = json.dumps(state()).encode(), "application/json"
            elif u.path == "/report":
                body, ctype = json.dumps(report()).encode(), "application/json"
            elif u.path.startswith("/icon/"):
                name = re.sub(r"[^a-z0-9-]", "", u.path[6:].removesuffix(".svg")) or "agent"
                f = os.path.join(ICONS, name + ".svg")
                body = open(f if os.path.exists(f) else os.path.join(ICONS, "agent.svg"), "rb").read()
                ctype = "image/svg+xml"
            elif u.path == "/detail":
                body, ctype = json.dumps(detail(parse_qs(u.query).get("id", [""])[0])).encode(), "application/json"
            else:
                body, ctype = open(PAGE, "rb").read(), "text/html; charset=utf-8"
        except Exception as e:  # keep the dashboard up; show the failure on the page
            body, ctype = json.dumps({"fatal": repr(e)}).encode(), "application/json"
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    if "--dump" in sys.argv:
        print(json.dumps(state(), indent=1)[:4000])
        sys.exit()
    print(f"Athena map on http://127.0.0.1:{PORT}  (ctrl+c quits)")
    ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()
