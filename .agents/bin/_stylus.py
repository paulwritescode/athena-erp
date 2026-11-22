"""Shared helpers for the Stylus agent tools in .agents/bin.

Every tool resolves the main repository from git's common directory, so a copy
run from a worktree still writes the one shared record in the main repo's
.agents/stylus/ folder.
"""
import json, os, shutil, subprocess, time


def main_root():
    here = os.path.dirname(os.path.abspath(__file__))
    common = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
                            cwd=here, capture_output=True, text=True).stdout.strip()
    return os.path.dirname(common) if common else os.path.dirname(os.path.dirname(here))


ROOT = main_root()
STY = os.path.join(ROOT, ".agents", "stylus")
WORKSPACE_PARENT = os.path.dirname(ROOT)


def path(name):
    return os.path.join(STY, name)


def now():
    return time.strftime("%Y-%m-%dT%H:%M:%S%z")


def append(name, rec):
    rec = {"ts": now(), **rec}
    with open(path(name), "a") as f:
        f.write(json.dumps(rec, ensure_ascii=False) + "\n")
    return rec


def read_jsonl(name, last=None):
    try:
        with open(path(name)) as f:
            rows = [json.loads(l) for l in f if l.strip()]
    except FileNotFoundError:
        return []
    return rows[-last:] if last else rows


def limits():
    with open(path("limits.json")) as f:
        return json.load(f)


def hc(*args, timeout=10):
    r = subprocess.run(["hc", *args], capture_output=True, text=True, timeout=timeout)
    return r.stdout


def terminals():
    try:
        return json.loads(hc("ls") or "{}").get("terminals", [])
    except ValueError:
        return []


def me():
    """Name (or address) of the terminal this tool runs in."""
    tid = os.environ.get("HC_TERMINAL_ID", "")
    for t in terminals():
        if t["address"] == tid:
            return t.get("name") or ("stylus" if tid == stylus_addr() else tid)
    return tid or "user"


def stylus_addr():
    try:
        return limits().get("stylus", "t6")
    except Exception:
        return "t6"


def mem_free_pct():
    out = subprocess.run(["memory_pressure"], capture_output=True, text=True).stdout
    for line in out.splitlines():
        if "free percentage" in line:
            return int(line.rsplit(":", 1)[1].strip().rstrip("%"))
    return 100


def disk_free_gb():
    return shutil.disk_usage("/").free / 1e9
