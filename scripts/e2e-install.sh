#!/bin/sh
# End-to-end check of the Athena install path (B11 slice 1):
#   athena init -> athena new [--logo] -> athena run ->
#   sign in as root -> rotate the root password -> authenticated calls.
#
# Throwaway only: a temp HOME (the install lock allows one server per HOME),
# a temp home directory, a test_athena_* database, and a loopback port.
# Nothing here touches the dev database or the real home.
#
# Usage from the repository root:
#   make build && ./scripts/e2e-install.sh
#
# The database server login comes from ATHENA_TEST_DATABASE_URL, falling back
# to ATHENA_DATABASE_URL from .env.local, exactly like `make test`. ADDR
# overrides the loopback address (default 127.0.0.1:3211) and SLUG the
# workspace slug (default test_athena_b11_slice1), so parallel runs can
# take distinct addresses and databases. KEEP=1 skips cleanup for
# inspection; the script prints the scratch paths in that case.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ATHENA="$ROOT/bin/athena"
ADDR="${ADDR:-127.0.0.1:3211}"
BASE="http://$ADDR"
SLUG="${SLUG:-test_athena_b11_slice1}"
# The workspace name is the slug in title case, so `athena new` derives
# the slug back exactly for any SLUG override.
NAME="$(printf '%s' "$SLUG" | tr '_' ' ' | awk '{for (i=1;i<=NF;i++) $i = toupper(substr($i,1,1)) substr($i,2)} 1')"

pass=0
fail=0
check() { # check <description> <command...>
  desc="$1"; shift
  if "$@" >/dev/null 2>&1; then
    pass=$((pass + 1)); printf 'ok   %s\n' "$desc"
  else
    fail=$((fail + 1)); printf 'FAIL %s\n' "$desc"
  fi
}
expect_fail() { # expect_fail <description> <needle> <command...>
  desc="$1"; needle="$2"; shift 2
  out="$("$@" 2>&1)" && { fail=$((fail + 1)); printf 'FAIL %s (expected an error, got success)\n' "$desc"; return; }
  case "$out" in
    *"$needle"*) pass=$((pass + 1)); printf 'ok   %s\n' "$desc" ;;
    *) fail=$((fail + 1)); printf 'FAIL %s (wanted %s, got: %s)\n' "$desc" "$needle" "$out" ;;
  esac
}

[ -x "$ATHENA" ] || { echo "bin/athena is missing; run 'make build' first." >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required." >&2; exit 1; }
case "$SLUG" in test_athena_*) ;; *) echo "refusing: slug must start with test_athena_" >&2; exit 1;; esac
if ! printf '%s' "$SLUG" | grep -qE '^[a-z][a-z0-9_]{0,47}$'; then
  echo "refusing: slug $SLUG is not a valid workspace slug" >&2; exit 1
fi
case "$SLUG" in *_) echo "refusing: slug must not end with an underscore" >&2; exit 1;; esac

# Server login, sourced like `make test` does. Never printed.
if [ -z "${ATHENA_TEST_DATABASE_URL:-}" ] && [ -f "$ROOT/.env.local" ]; then
  # shellcheck disable=SC1091
  set -a; . "$ROOT/.env.local"; set +a
fi
DSN="${ATHENA_TEST_DATABASE_URL:-${ATHENA_DATABASE_URL:-}}"
[ -n "$DSN" ] || { echo "set ATHENA_TEST_DATABASE_URL or ATHENA_DATABASE_URL in .env.local" >&2; exit 1; }
case "$DSN" in *@tcp\(*\)/*) ;; *) echo "this check needs a MariaDB DSN of the form user:pass@tcp(host:port)/db" >&2; exit 1;; esac
USERINFO="${DSN%@*}"; TAIL="${DSN##*@}"
DBUSER="${USERINFO%%:*}"; DBPASS="${USERINFO#*:}"
HOSTPORT="${TAIL%%/*}"; HOSTPORT="${HOSTPORT#tcp(}"; HOSTPORT="${HOSTPORT%)}"
DBHOST="${HOSTPORT%%:*}"; DBPORT="${HOSTPORT##*:}"

if command -v mariadb >/dev/null; then DBCLI=mariadb; else DBCLI=mysql; fi
db() { "$DBCLI" --protocol=TCP -h "$DBHOST" -P "$DBPORT" -u "$DBUSER" -p"$DBPASS" "$@"; }

FAKEHOME="$(mktemp -d /tmp/athena-b11-fakehome.XXXXXX)"
SCRATCH="$(mktemp -d /tmp/athena-b11-e2e.XXXXXX)"
HOMEDIR="$SCRATCH/home"
WS="$HOMEDIR/$SLUG"
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$SERVER_PID" 2>/dev/null || break; sleep 1; done
    kill -9 "$SERVER_PID" 2>/dev/null || true
  fi
  if [ "${KEEP:-0}" = "1" ]; then
    printf 'KEEP=1: fake HOME %s, scratch %s, database %s\n' "$FAKEHOME" "$SCRATCH" "$SLUG"
    return
  fi
  db -e "DROP DATABASE IF EXISTS \`$SLUG\`" 2>/dev/null || true
  rm -rf "$SCRATCH" "$FAKEHOME"
}
trap cleanup EXIT INT TERM

export HOME="$FAKEHOME"
export XDG_CONFIG_HOME="$FAKEHOME/.config"

# From here on, failures are recorded by check/expect_fail instead of
# aborting, so one run reports every step. The exit status comes from
# the final tally.
set +e

printf '\n== init ==\n'
ATHENA_SERVER_PASSWORD="$DBPASS" "$ATHENA" init "$HOMEDIR" --host "$DBHOST" --port "$DBPORT" --user "$DBUSER"
check "home files exist" test -f "$HOMEDIR/athena-home.yaml"
[ "$(stat -f %A "$HOMEDIR/.env.local" 2>/dev/null || stat -c %a "$HOMEDIR/.env.local")" = "600" ] \
  && { pass=$((pass + 1)); printf 'ok   home .env.local is owner-only\n'; } \
  || { fail=$((fail + 1)); printf 'FAIL home .env.local is owner-only\n'; }
check "home stores the server login" grep -q ATHENA_SERVER_URL "$HOMEDIR/.env.local"
expect_fail "init twice is refused" "already contains a home" \
  env ATHENA_SERVER_PASSWORD="$DBPASS" "$ATHENA" init "$HOMEDIR" --host "$DBHOST" --port "$DBPORT" --user "$DBUSER"

printf '\n== new ==\n'
# A 1x1 PNG for --logo.
python3 -c "import base64; open('$SCRATCH/logo.png','wb').write(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='))"
cd "$HOMEDIR"
"$ATHENA" new "$NAME" --logo "$SCRATCH/logo.png"
check "workspace directory exists" test -f "$WS/athena.yaml"
check "workspace is a git repo on main" sh -c "git -C '$WS' symbolic-ref --short HEAD | grep -q '^main$'"
check "workspace scaffolding exists" sh -c "test -f '$WS/README.md' && test -f '$WS/AGENTS.md' && test -d '$WS/record_types'"
check "database recorded in manifest" grep -q "name: $SLUG" "$WS/athena.yaml"
db_has_workspace() { db -e 'SHOW TABLES' "$SLUG" 2>/dev/null | grep -q athena_workspace; }
check "database exists on the server" db_has_workspace
IDENTITY_OUT="$("$ATHENA" workspace identity show --workspace "$SLUG")"
case "$IDENTITY_OUT" in *logoLightUrl*|*logo_light_url*) pass=$((pass + 1)); printf 'ok   --logo stored on the workspace\n' ;; *) fail=$((fail + 1)); printf 'FAIL --logo stored on the workspace (got: %s)\n' "$IDENTITY_OUT" ;; esac
expect_fail "duplicate name is refused" "already exists" "$ATHENA" new "$NAME"
expect_fail "driver mismatch is refused" "does not match the home driver" "$ATHENA" new --driver postgres "Another Name"
expect_fail "missing logo is refused" "read " "$ATHENA" new --logo "$SCRATCH/no-such.png" "Logo Missing"
mkdir -p "$SCRATCH/nowhere" && cd "$SCRATCH/nowhere"
expect_fail "new outside a home is refused" "no Athena home" "$ATHENA" new "Lost Workspace"
cd "$HOMEDIR"
expect_fail "run from the home needs --workspace" "not a workspace" "$ATHENA" run --addr "$ADDR"
expect_fail "run rejects a positional argument" "unexpected argument" "$ATHENA" run "$SLUG" --addr "$ADDR"

printf '\n== run ==\n'
"$ATHENA" run --workspace "$SLUG" --addr "$ADDR" >"$SCRATCH/server.log" 2>&1 &
SERVER_PID=$!
i=0
until curl -fsS "$BASE/healthz" 2>/dev/null | grep -q '"database":"Connected"'; do
  i=$((i + 1))
  if [ "$i" -ge 90 ]; then echo "server did not come up; log:" >&2; cat "$SCRATCH/server.log" >&2; exit 1; fi
  sleep 2
done
pass=$((pass + 1)); printf 'ok   server healthy with a connected database\n'
check "desk page is served" sh -c "curl -fsS '$BASE/' | grep -q '<div id=\"app\">'"
cd "$WS"
expect_fail "second server on the same HOME is refused" "already running" "$ATHENA" run --addr 127.0.0.1:3212
# First-boot root handoff: the database password seeds the root account, or a
# generated one is printed once when it cannot.
ROOTPASS="$DBPASS"
if grep -q "shown once: " "$SCRATCH/server.log"; then
  ROOTPASS="$(sed -n 's/.*shown once: //p' "$SCRATCH/server.log" | tail -1)"
  pass=$((pass + 1)); printf 'ok   unusable database password produced a printed-once root password\n'
else
  check "usable database password seeds root silently" grep -q "connected; metadata tables are ready" "$SCRATCH/server.log"
fi

printf '\n== sign in as root ==\n'
cd "$SCRATCH"
JAR="$SCRATCH/jar"; : >"$JAR"
ROOTPW_ENV() { ROOTPW="$1"; export ROOTPW; }
ROOTPW_ENV "$ROOTPASS"
python3 - "$SCRATCH/login.json" <<'EOF'
import json, os, sys
json.dump({"username": "administrator", "password": os.environ["ROOTPW"]}, open(sys.argv[1], "w"))
EOF
curl -fsS -c "$JAR" -H 'content-type: application/json' -H 'accept: application/json' \
  -d @"$SCRATCH/login.json" "$BASE/athena.v1.AuthService/Login" -o "$SCRATCH/login.out"
python3 - "$SCRATCH/login.out" "$SCRATCH/csrf" <<'EOF'
import json, sys
body = json.load(open(sys.argv[1]))
session = body.get("session", {})
assert session.get("authenticated") is True, body
assert session.get("username") == "administrator", body
assert session.get("csrfToken"), body
open(sys.argv[2], "w").write(session["csrfToken"])
EOF
check "root signs in over the API" test -s "$SCRATCH/csrf"
CSRF="$(cat "$SCRATCH/csrf" 2>/dev/null || true)"
curl -fsS -b "$JAR" -H 'content-type: application/json' -H 'accept: application/json' \
  -H "X-CSRF-Token: $CSRF" -d '{}' "$BASE/athena.v1.AuthService/GetSession" -o "$SCRATCH/session.out"
check "session cookie authenticates" sh -c "grep -q '\"authenticated\":true' '$SCRATCH/session.out'"
NEWPASS="B11-slice1-rotated-$RANDOM$RANDOM"
export ROOTPW_OLD="$ROOTPASS"
ROOTPW_ENV "$NEWPASS"
python3 - "$SCRATCH/rotate.json" <<'EOF'
import json, os, sys
json.dump({"currentPassword": os.environ["ROOTPW_OLD"], "newPassword": os.environ["ROOTPW"]}, open(sys.argv[1], "w"))
EOF
curl -fsS -b "$JAR" -H 'content-type: application/json' -H 'accept: application/json' \
  -H "X-CSRF-Token: $CSRF" -d @"$SCRATCH/rotate.json" "$BASE/athena.v1.AuthService/ChangePassword" -o /dev/null
check "root rotates the password" test $? -eq 0
: >"$JAR"
python3 - "$SCRATCH/login2.json" <<'EOF'
import json, os, sys
json.dump({"username": "administrator", "password": os.environ["ROOTPW"]}, open(sys.argv[1], "w"))
EOF
curl -fsS -c "$JAR" -H 'content-type: application/json' -H 'accept: application/json' \
  -d @"$SCRATCH/login2.json" "$BASE/athena.v1.AuthService/Login" -o "$SCRATCH/login2.out"
python3 - "$SCRATCH/login2.out" "$SCRATCH/csrf2" <<'EOF'
import json, sys
body = json.load(open(sys.argv[1]))
assert body.get("session", {}).get("authenticated") is True, body
open(sys.argv[2], "w").write(body["session"]["csrfToken"])
EOF
check "root signs in with the rotated password" test -s "$SCRATCH/csrf2"
CSRF2="$(cat "$SCRATCH/csrf2" 2>/dev/null || true)"
curl -fsS -b "$JAR" -H 'content-type: application/json' -H 'accept: application/json' \
  -H "X-CSRF-Token: $CSRF2" -d '{}' "$BASE/athena.v1.RecordTypeService/ListRecordTypes" -o /dev/null
check "root lists Record Types" test $? -eq 0

printf '\n== CLI equivalents ==\n'
check "operator lists users" sh -c "cd '$WS' && '$ATHENA' user list >/dev/null"
check "migrate status runs" sh -c "cd '$WS' && '$ATHENA' migrate status >/dev/null"
INVITE_OUT="$(cd "$WS" && "$ATHENA" user invite b11owner --owner --workspace "$SLUG")"
case "$INVITE_OUT" in *"Invite challenge, shown once"*) pass=$((pass + 1)); printf 'ok   operator invites the first owner\n' ;; *) fail=$((fail + 1)); printf 'FAIL operator invites the first owner (got: %s)\n' "$INVITE_OUT" ;; esac

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
