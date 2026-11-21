#!/bin/zsh
# Master status snapshot for the Athena beta orchestration (read-only).
# Prints merged work, open worktrees, worker states and the task list.
R=/Users/iampaulking/projects/duchess/athena-erp
cd $R || exit 1
git fetch -q origin 2>/dev/null
echo "== Merged to main today (features and fixes)"
git log --since="$(date +%Y-%m-%d) 00:00" --format='%h %s' origin/main | grep -E ' (feat|fix|test|build)' 
echo
echo "== Open worktrees (work not yet merged)"
git worktree list | grep -v "\[main\]" | while read -r wt sha branch; do
  echo "$branch  $(git -C "$wt" status -s | wc -l | tr -d ' ') changed files  $wt"
done
echo
echo "== Workers"
hc ls 2>/dev/null | python3 -c '
import sys, json
for t in json.load(sys.stdin)["terminals"]:
    if t.get("agent") in ("muse", "codex"):
        print(t["address"], t["agent"], t.get("status"), t["cwd"].split("/")[-1])'
echo
echo "== Beta task list"
echo "done: $(grep -c '^- \[x\]' .kiro/specs/athena-beta/tasks.md)  open: $(grep -c '^- \[ \]' .kiro/specs/athena-beta/tasks.md)"
grep '^- \[x\]' .kiro/specs/athena-beta/tasks.md | sed -E 's/^- \[x\] \*\*([^*]+)\*\*.*/  done  \1/'
grep '^- \[ \]' .kiro/specs/athena-beta/tasks.md | sed -E 's/^- \[ \] \*\*([^*]+)\*\*.*/  open  \1/'
