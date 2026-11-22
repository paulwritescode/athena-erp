#!/usr/bin/env python3
"""Live task and agent dashboard. Run: python3 .agents/dashboard.py   (q or ctrl+c quits)

Reads, every 2 seconds: .kiro/specs/athena-beta/tasks.md (task list and merged
state), `hc ls` (terminals: master and workers), and `git worktree list` (branch
and unmerged commits per worktree). Nothing is cached, so a new worker, worktree
or checked-off task appears on the next refresh.
"""
import json, os, re, shutil, subprocess, sys, time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TASKS = os.path.join(ROOT, ".kiro/specs/athena-beta/tasks.md")
REFRESH = 2.0

C = {"g": "\033[32m", "y": "\033[33m", "b": "\033[34m", "r": "\033[31m",
     "d": "\033[2m", "B": "\033[1m", "x": "\033[0m", "c": "\033[36m"}


def run(args, cwd=ROOT):
    try:
        return subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=5).stdout
    except Exception:
        return ""


def read_tasks():
    out, section = [], ""
    try:
        lines = open(TASKS, encoding="utf-8").read().splitlines()
    except OSError:
        return out
    for ln in lines:
        if ln.startswith("## "):
            section = ln[3:].split(".")[0].strip() if re.match(r"## [A-E]\.", ln) else ""
            continue
        m = re.match(r"- \[( |x)\] \*\*([A-Z]+\d+[a-z]?)\. ([^*]+?)\*\*", ln)
        if m and section:
            out.append({"done": m.group(1) == "x", "id": m.group(2), "title": m.group(3).rstrip(". "), "sec": section})
    return out


def worktrees():
    res = []
    cur = {}
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
        dirty = run(["git", "status", "--porcelain"], cwd=w["worktree"])
        w["dirty"] = len([l for l in dirty.splitlines() if not l.startswith("??")])
    return res


def terminals():
    try:
        d = json.loads(run(["hc", "ls"]) or "{}")
    except ValueError:
        return []
    return d.get("terminals", [])


def task_key(path):
    base = os.path.basename(path.rstrip("/"))
    m = re.match(r"athena-erp-([a-z0-9]+)", base)
    return m.group(1).upper().replace("B9", "B9") if m else None


def state_for(task, wts, workers):
    tid = task["id"].lower()
    if task["done"]:
        return "MERGED", "g"
    for t in workers:
        if task_key(t.get("cwd", "")) and task_key(t["cwd"]).lower() == tid:
            if t.get("status") == "working":
                return "RUNNING", "y"
            ahead = [w for w in wts if task_key(w["worktree"]) == task_key(t["cwd"])]
            if ahead and ahead[0]["ahead"]:
                return "READY TO MERGE", "c"
            return "WAITING", "y"
    for w in wts:
        k = task_key(w["worktree"])
        if k and (k.lower() == tid or (tid == "b9" and k.lower().startswith("b9-"))):
            if w["ahead"]:
                return "READY TO MERGE", "c"
            return "STARTED", "b"
    return "NOT STARTED", "d"


def fit(s, n):
    s = re.sub(r"\s+", " ", s)
    return s if len(s) <= n else s[: n - 1] + "…"


SECTIONS = {"A": "FOUNDATION", "B": "PLATFORM", "C": "KERNEL", "D": "PRODUCT", "E": "BETA RELEASE"}
ICON = {"MERGED": "✔", "RUNNING": "▶", "WAITING": "◔", "READY TO MERGE": "◆", "STARTED": "◐", "NOT STARTED": "○"}


def pad(s, n):
    return fit(s, n).ljust(n)


def render():
    cols, rows = shutil.get_terminal_size((100, 40))
    tasks, wts, terms = read_tasks(), worktrees(), terminals()
    workers = [t for t in terms if t.get("agent") == "muse"]
    masters = [t for t in terms if t.get("agent") != "muse" and any(x.get("parent") == t["address"] for x in terms)]
    st = {t["id"]: state_for(t, wts, workers) for t in tasks}
    who = {}
    for w in workers:
        k = task_key(w.get("cwd", ""))
        for t in tasks:
            if k and k.lower() == t["id"].lower():
                who[t["id"]] = w
    count = {}
    for s_, _ in st.values():
        count[s_] = count.get(s_, 0) + 1
    total = len(tasks) or 1
    merged = count.get("MERGED", 0)
    bar_w = max(10, min(60, cols - 40))
    fill = int(bar_w * merged / total)
    L = [f"{C['B']}ATHENA BETA MAP{C['x']}  {time.strftime('%H:%M:%S')}  main {run(['git','rev-parse','--short','main']).strip()}   "
         f"[{C['g']}{'█' * fill}{C['x']}{'░' * (bar_w - fill)}] {merged}/{total} merged"]
    L.append("  ".join(f"{C[c]}{ICON[s_]} {s_.lower()} {count[s_]}{C['x']}" for s_, c in
                      [("RUNNING", "y"), ("WAITING", "y"), ("READY TO MERGE", "c"), ("STARTED", "b"), ("MERGED", "g"), ("NOT STARTED", "d")] if s_ in count))
    m = masters[0]["address"] if masters else "-"
    L.append(f"{C['B']}MASTER {m}{C['x']} " + "".join(
        f"{C['d']}──{C['x']}{C['y' if w.get('status') == 'working' else 'g']}● {w.get('name') or w['address']}{C['x']} " for w in workers))
    W = 30
    per = max(1, (cols - 2) // (W + 1))
    for sec, name in SECTIONS.items():
        ts = [t for t in tasks if t["sec"] == sec]
        if not ts:
            continue
        done = sum(1 for t in ts if t["done"])
        L.append(f"{C['B']}{C['c']}━━ {sec} {name}{C['x']} {C['d']}{done}/{len(ts)} merged {'━' * max(0, cols - len(name) - 24)}{C['x']}")
        for r in range(0, len(ts), per):
            row = ts[r:r + per]
            l1, l2, l3 = [], [], []
            for t in row:
                s_, col = st[t["id"]]
                w = who.get(t["id"])
                tag = f" {w['address']} {w.get('status', '')}" if w else ""
                l1.append(f"{C[col]}{ICON[s_]} {C['B']}{pad(t['id'], 4)}{C['x']}{C[col]}{pad(s_.lower() + tag, W - 7)}{C['x']}")
                l2.append(f"{C[col] if s_ != 'NOT STARTED' else C['d']}{pad(t['title'], W - 1)}{C['x']} ")
                br = next((x["branch"] for x in wts if task_key(x["worktree"]) and task_key(x["worktree"]).lower() == t["id"].lower()), "")
                l3.append(f"{C['d']}{pad(br, W - 1)}{C['x']} ")
            L.append(" ".join(l1)); L.append("".join(l2)); L.append("".join(l3))
    return "\033[H\033[J" + "\n".join(fit_ansi(l, cols) for l in L[: rows - 1])


def fit_ansi(line, cols):
    plain = re.sub(r"\033\[[0-9;]*m", "", line)
    return line if len(plain) <= cols else plain[: cols - 1] + "…"


def main():
    sys.stdout.write("\033[?1049h\033[?25l")
    try:
        while True:
            sys.stdout.write(render())
            sys.stdout.flush()
            time.sleep(REFRESH)
    except KeyboardInterrupt:
        pass
    finally:
        sys.stdout.write("\033[?25h\033[?1049l")


if __name__ == "__main__":
    main()
