#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_HOME="$(mktemp -d)"
BIN="$TMP_HOME/bin/pm2-go"
E2E_SLOW="${E2E_SLOW:-0}"
WEB_PID=""

log() {
	printf '== %s ==\n' "$*" >&2
}

fail() {
	printf 'e2e failed: %s\n' "$*" >&2
	exit 1
}

run_pm2() {
	HOME="$TMP_HOME" "$BIN" "$@"
}

capture_pm2() {
	HOME="$TMP_HOME" "$BIN" "$@" 2>&1
}

assert_contains() {
	local output="$1"
	local needle="$2"

	if ! printf '%s\n' "$output" | grep -Fq "$needle"; then
		printf '%s\n' "$output" >&2
		fail "expected output to contain '$needle'"
	fi
}

assert_not_contains() {
	local output="$1"
	local needle="$2"

	if printf '%s\n' "$output" | grep -Fq "$needle"; then
		printf '%s\n' "$output" >&2
		fail "expected output not to contain '$needle'"
	fi
}

assert_line_count() {
	local output="$1"
	local needle="$2"
	local expected="$3"
	local matches
	local actual

	matches="$(printf '%s\n' "$output" | grep -F "$needle" || true)"
	actual="$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d '[:space:]')"

	if [[ "$actual" != "$expected" ]]; then
		printf '%s\n' "$output" >&2
		fail "expected '$needle' to appear on $expected line(s), found $actual"
	fi
}

process_parent_pid() {
	local output="$1"
	local name="$2"

	printf '%s\n' "$output" | awk -F'│' -v name="$name" '
		index($0, name) {
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", $5)
			print $5
			exit
		}
	'
}

assert_parent_is_daemon() {
	local output="$1"
	local name="$2"
	local expected
	local actual

	expected="$(cat "$TMP_HOME/.pm2-go/daemon.pid")"
	actual="$(process_parent_pid "$output" "$name")"

	if [[ -z "$actual" || "$actual" != "$expected" ]]; then
		printf '%s\n' "$output" >&2
		fail "expected $name parent pid to be daemon $expected, found ${actual:-missing}"
	fi
}

assert_file_empty() {
	local file="$1"
	local size

	[[ -f "$file" ]] || fail "expected file to exist: $file"
	size="$(wc -c <"$file" | tr -d '[:space:]')"
	[[ "$size" == "0" ]] || fail "expected $file to be empty, found $size byte(s)"
}

assert_file_not_empty() {
	local file="$1"
	local size

	[[ -f "$file" ]] || fail "expected file to exist: $file"
	size="$(wc -c <"$file" | tr -d '[:space:]')"
	[[ "$size" != "0" ]] || fail "expected $file to contain data"
}

assert_file_not_contains() {
	local file="$1"
	local needle="$2"

	[[ -f "$file" ]] || fail "expected file to exist: $file"
	if grep -Fq "$needle" "$file"; then
		cat "$file" >&2
		fail "expected $file not to contain '$needle'"
	fi
}

assert_command_fails() {
	local output
	local status

	set +e
	output="$(capture_pm2 "$@")"
	status=$?
	set -e

	if [[ "$status" == "0" ]]; then
		printf '%s\n' "$output" >&2
		fail "expected command to fail: pm2-go $*"
	fi

	printf '%s\n' "$output"
}

wait_for_file_not_empty() {
	local file="$1"
	local timeout="${2:-10}"

	for _ in $(seq 1 "$timeout"); do
		if [[ -s "$file" ]]; then
			return
		fi
		sleep 1
	done

	fail "timed out waiting for file to contain data: $file"
}

wait_for_file_contains() {
	local file="$1"
	local needle="$2"
	local timeout="${3:-10}"

	for _ in $(seq 1 "$timeout"); do
		if [[ -f "$file" ]] && grep -Fq "$needle" "$file"; then
			return
		fi
		sleep 1
	done

	[[ -f "$file" ]] && cat "$file" >&2
	fail "timed out waiting for $file to contain '$needle'"
}

wait_for_ls_contains() {
	local name="$1"
	local state="$2"
	local timeout="${3:-10}"
	local output

	for _ in $(seq 1 "$timeout"); do
		output="$(run_pm2 ls)"
		if printf '%s\n' "$output" | grep -Fq "$name" && printf '%s\n' "$output" | grep -Fq "$state"; then
			printf '%s\n' "$output"
			return
		fi
		sleep 1
	done

	printf '%s\n' "$output" >&2
	fail "timed out waiting for $name to be $state"
}

wait_for_daemon_log() {
	local needle="$1"
	local timeout="${2:-10}"
	local daemon_log="$TMP_HOME/.pm2-go/daemon.log"

	for _ in $(seq 1 "$timeout"); do
		if [[ -f "$daemon_log" ]] && grep -Fq "$needle" "$daemon_log"; then
			return
		fi
		sleep 1
	done

	[[ -f "$daemon_log" ]] && cat "$daemon_log" >&2
	fail "timed out waiting for daemon log to contain '$needle'"
}

wait_for_pid_exit() {
	local pid="$1"
	local timeout="${2:-20}"

	for _ in $(seq 1 "$timeout"); do
		if ! kill -0 "$pid" >/dev/null 2>&1; then
			return
		fi
		sleep 0.1
	done

	fail "timed out waiting for pid $pid to exit"
}

free_port() {
	python3 - <<'PY'
import socket

with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    print(sock.getsockname()[1])
PY
}

wait_for_web() {
	local port="$1"

	for _ in $(seq 1 30); do
		if python3 - "$port" <<'PY' >/dev/null 2>&1
import http.client
import sys

conn = http.client.HTTPConnection("127.0.0.1", int(sys.argv[1]), timeout=1)
conn.request("GET", "/login")
resp = conn.getresponse()
sys.exit(0 if resp.status == 200 else 1)
PY
		then
			return
		fi
		sleep 0.2
	done

	fail "timed out waiting for web dashboard on port $port"
}

web_process_json() {
	local port="$1"

	python3 - "$port" <<'PY'
from http.cookies import SimpleCookie
import http.client
import json
import sys
import urllib.parse

port = int(sys.argv[1])

conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request("GET", "/api/processes")
resp = conn.getresponse()
if resp.status != 401:
    raise SystemExit(f"expected unauthenticated API to return 401, got {resp.status}")
resp.read()
conn.close()

body = urllib.parse.urlencode({"token": "web-e2e-token"})
conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request("POST", "/auth/login", body, {"Content-Type": "application/x-www-form-urlencoded"})
resp = conn.getresponse()
cookies = SimpleCookie()
for header, value in resp.getheaders():
    if header.lower() == "set-cookie":
        cookies.load(value)
if resp.status != 303 or "pm2_go_web_session" not in cookies or "pm2_go_web_csrf" not in cookies:
    raise SystemExit(f"expected login redirect with cookie, got {resp.status}")
resp.read()
conn.close()

cookie = "; ".join(f"{morsel.key}={morsel.value}" for morsel in cookies.values())
csrf = cookies["pm2_go_web_csrf"].value

conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request("GET", "/api/processes", headers={"Cookie": cookie})
resp = conn.getresponse()
payload = resp.read().decode()
if resp.status != 200:
    raise SystemExit(f"expected authenticated API to return 200, got {resp.status}: {payload}")
data = json.loads(payload)
if not data.get("processes"):
    raise SystemExit("expected at least one process")
process = data["processes"][0]
process_id = process["id"]

conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request("GET", f"/api/processes/{process_id}/metrics", headers={"Cookie": cookie})
resp = conn.getresponse()
metrics_payload = resp.read().decode()
if resp.status != 200 or "points" not in metrics_payload:
    raise SystemExit(f"expected metrics response, got {resp.status}: {metrics_payload}")
conn.close()

conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request("GET", f"/api/processes/{process_id}/logs?stream=out&tail=5", headers={"Cookie": cookie})
resp = conn.getresponse()
logs_payload = resp.read().decode()
if resp.status != 200 or "lines" not in logs_payload:
    raise SystemExit(f"expected logs response, got {resp.status}: {logs_payload}")
conn.close()

conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
conn.request(
    "POST",
    f"/api/processes/{process_id}/actions",
    json.dumps({"action": "restart"}),
    {
        "Content-Type": "application/json",
        "Cookie": cookie,
        "X-CSRF-Token": csrf,
    },
)
resp = conn.getresponse()
action_payload = resp.read().decode()
if resp.status != 200 or '"success":true' not in action_payload:
    raise SystemExit(f"expected restart action success, got {resp.status}: {action_payload}")
conn.close()

print(json.dumps(data, sort_keys=True))
PY
}

capture_logs_for() {
	local name="$1"
	local output_file="$TMP_HOME/logs-$name.out"
	local pid

	run_pm2 logs -l 5 "$name" >"$output_file" 2>&1 &
	pid=$!
	sleep 2
	kill "$pid" >/dev/null 2>&1 || true
	wait "$pid" >/dev/null 2>&1 || true
	cat "$output_file"
}

write_autorestart_ecosystem() {
	cat >"$TMP_HOME/autorestart.json" <<'JSON'
[
  {
    "name": "autorestart-test",
    "args": ["-c", "import time, sys; time.sleep(0.2); sys.exit(2)"],
    "autorestart": true,
    "cwd": ".",
    "executable_path": "python3"
  }
]
JSON
}

write_restart_limit_ecosystem() {
	cat >"$TMP_HOME/restart-limit.json" <<'JSON'
[
  {
    "name": "restart-limit-test",
    "args": ["-c", "import time, sys; time.sleep(0.1); sys.exit(2)"],
    "autorestart": true,
    "cwd": ".",
    "executable_path": "python3",
    "max_restarts": 1,
    "min_uptime": 1000,
    "exp_backoff_restart_delay": 100
  }
]
JSON
}

write_delayed_restart_ecosystem() {
	cat >"$TMP_HOME/delayed-restart.json" <<'JSON'
[
  {
    "name": "delayed-restart-test",
    "args": ["-c", "import time, sys; time.sleep(0.1); sys.exit(2)"],
    "autorestart": true,
    "cwd": ".",
    "executable_path": "python3",
    "restart_delay": 2000
  }
]
JSON
}

write_ordered_logs_ecosystem() {
	cat >"$TMP_HOME/ordered-logs.json" <<'JSON'
[
  {
    "name": "ordered-logs-test",
    "args": ["-c", "import sys, time; print('mixed-out-1', flush=True); time.sleep(0.2); print('mixed-err-1', file=sys.stderr, flush=True); time.sleep(0.2); print('mixed-out-2', flush=True); time.sleep(0.2)"],
    "autorestart": false,
    "cwd": ".",
    "executable_path": "python3"
  }
]
JSON
}

write_memory_restart_ecosystem() {
	cat >"$TMP_HOME/memory-restart.json" <<'JSON'
[
  {
    "name": "memory-restart-test",
    "args": ["-c", "import time; data = bytearray(8 * 1024 * 1024); time.sleep(20)"],
    "autorestart": false,
    "cwd": ".",
    "executable_path": "python3",
    "max_memory_restart": 1048576
  }
]
JSON
}

write_health_check_ecosystem() {
	cat >"$TMP_HOME/health-check.json" <<'JSON'
[
  {
    "name": "health-check-test",
    "args": ["-c", "import time; time.sleep(20)"],
    "autorestart": false,
    "cwd": ".",
    "executable_path": "python3",
    "health_check_url": "http://127.0.0.1:9/unhealthy",
    "health_check_interval": 500,
    "health_check_timeout": 200
  }
]
JSON
}

write_watch_ecosystem() {
	mkdir -p "$TMP_HOME/watch-cwd"
	printf 'initial\n' >"$TMP_HOME/watch-cwd/watched.txt"
	cat >"$TMP_HOME/watch.json" <<JSON
[
  {
    "name": "watch-test",
    "args": ["-c", "import time; time.sleep(20)"],
    "autorestart": false,
    "cwd": "$TMP_HOME/watch-cwd",
    "executable_path": "python3",
    "watch": true,
    "watch_interval": 500
  }
]
JSON
}

write_cron_ecosystem() {
	cat >"$TMP_HOME/cron.json" <<'JSON'
[
  {
    "name": "cron-test",
    "args": ["-c", "import time; time.sleep(20)"],
    "autorestart": false,
    "cwd": ".",
    "executable_path": "python3",
    "cron_restart": "* * * * *"
  }
]
JSON
}

write_state_restore_ecosystem() {
	cat >"$TMP_HOME/state-restore.json" <<'JSON'
[
  {
    "name": "state-restore",
    "args": ["-c", "import time; time.sleep(30)"],
    "autorestart": false,
    "cwd": ".",
    "executable_path": "python3"
  }
]
JSON
}

write_env_ecosystem() {
	cat >"$TMP_HOME/env.json" <<'JSON'
[
  {
    "name": "env-test",
    "args": ["-c", "import os, time; print(os.environ.get('PM2_GO_E2E_ENV')); print(os.environ.get('PM2_GO_E2E_CALLER_ENV')); print(os.environ.get('PM2_GO_PROFILE_ENV')); time.sleep(20)"],
    "autorestart": false,
    "cwd": ".",
    "env": {
      "PM2_GO_E2E_ENV": "ecosystem-value"
    },
    "env_production": {
      "PM2_GO_E2E_ENV": "production-value",
      "PM2_GO_PROFILE_ENV": "profile-only"
    },
    "executable_path": "python3"
  }
]
JSON
}

cleanup() {
	set +e
	if [[ -n "$WEB_PID" ]]; then
		kill "$WEB_PID" >/dev/null 2>&1 || true
		wait "$WEB_PID" >/dev/null 2>&1 || true
	fi
	if [[ -x "$BIN" ]]; then
		HOME="$TMP_HOME" "$BIN" delete all >/dev/null 2>&1
		HOME="$TMP_HOME" "$BIN" kill >/dev/null 2>&1
	fi
	rm -rf "$TMP_HOME"
}
trap cleanup EXIT

command -v go >/dev/null 2>&1 || fail "go is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"

cd "$ROOT_DIR"
mkdir -p "$(dirname "$BIN")"

log "build cli"
go build -o "$BIN" ./cmd/pm2-go

log "status before daemon"
status_output="$(capture_pm2 status)"
assert_contains "$status_output" "PM2 Daemon Not Running"

log "isolated daemon environment"
PM2_GO_DAEMON_ONLY=daemon-only run_pm2 -d >/dev/null
run_pm2 start -- python3 -c 'import os, time; print("daemon-env=" + str(os.environ.get("PM2_GO_DAEMON_ONLY"))); time.sleep(20)' >/dev/null
isolated_log="$TMP_HOME/.pm2-go/logs/python3-out.log"
wait_for_file_contains "$isolated_log" "daemon-env=None" 10
run_pm2 delete all >/dev/null
run_pm2 kill >/dev/null

log "start ecosystem"
run_pm2 start examples/ecosystem.json >/dev/null
sleep 1

ecosystem_ls="$(wait_for_ls_contains "python-test" "online")"
assert_line_count "$ecosystem_ls" "python-test" 1
assert_parent_is_daemon "$ecosystem_ls" "python-test"

log "web dashboard"
web_port="$(free_port)"
web_log="$TMP_HOME/web.log"
PM2_GO_WEB_TOKEN=web-e2e-token HOME="$TMP_HOME" "$BIN" web --port "$web_port" >"$web_log" 2>&1 &
WEB_PID=$!
wait_for_web "$web_port"
web_json="$(web_process_json "$web_port")"
assert_contains "$web_json" '"name": "python-test"'
assert_contains "$web_json" '"status": "online"'
assert_not_contains "$web_json" '"env"'
kill "$WEB_PID" >/dev/null 2>&1 || true
wait "$WEB_PID" >/dev/null 2>&1 || true
WEB_PID=""

log "status after daemon start"
status_output="$(capture_pm2 status)"
assert_contains "$status_output" "PM2 Daemon Running"
assert_contains "$status_output" "PID:"
status_json="$(run_pm2 status --json)"
assert_contains "$status_json" '"running": true'
ls_json="$(run_pm2 ls --json)"
assert_contains "$ls_json" '"name": "python-test"'
assert_contains "$ls_json" '"status": "online"'

log "config set and print"
config_output="$(capture_pm2 config set logrotate true)"
assert_contains "$config_output" "LogRotate has been set to true"
config_output="$(capture_pm2 config set logrotate_max_files 3)"
assert_contains "$config_output" "LogRotateMaxFiles has been set to 3"
config_output="$(capture_pm2 config set logrotate_size 1M)"
assert_contains "$config_output" "LogRotateSize has been set to 1048576 bytes"
config_output="$(run_pm2 config)"
assert_contains "$config_output" "log_rotate: true"
assert_contains "$config_output" "log_rotate_max_files: 3"
assert_contains "$config_output" "log_rotate_size: 1048576"

log "startup unit generation"
run_pm2 startup --unit-name pm2-go-e2e --user --output "$TMP_HOME/pm2-go.service" >/dev/null
wait_for_file_contains "$TMP_HOME/pm2-go.service" "Description=PM2-GO process manager (pm2-go-e2e)" 5
wait_for_file_contains "$TMP_HOME/pm2-go.service" "Environment=PM2_GO_HOME=$TMP_HOME/.pm2-go" 5
wait_for_file_contains "$TMP_HOME/pm2-go.service" "WantedBy=default.target" 5

log "describe process"
describe_output="$(capture_pm2 describe python-test)"
assert_contains "$describe_output" "Process with id"
assert_contains "$describe_output" "python-test"
assert_contains "$describe_output" "cron expression"
describe_json="$(run_pm2 describe python-test --json)"
assert_contains "$describe_json" '"name": "python-test"'
assert_contains "$describe_json" '"cron_restart": "* * * * *"'

log "logs command"
stdout_log="$TMP_HOME/.pm2-go/logs/python-test-out.log"
wait_for_file_not_empty "$stdout_log" 10
logs_output="$(capture_logs_for python-test)"
assert_contains "$logs_output" "[TAILING]"
assert_contains "$logs_output" "python-test"
assert_contains "$logs_output" "0"

log "ordered combined logs"
write_ordered_logs_ecosystem
run_pm2 start "$TMP_HOME/ordered-logs.json" >/dev/null
combined_log="$TMP_HOME/.pm2-go/logs/ordered-logs-test-combined.jsonl"
wait_for_file_contains "$combined_log" "mixed-out-2" 10
python3 - "$combined_log" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    entries = [json.loads(line) for line in handle if line.strip()]

observed = [(entry["stream"], entry["line"]) for entry in entries[-3:]]
expected = [
    ("stdout", "mixed-out-1"),
    ("stderr", "mixed-err-1"),
    ("stdout", "mixed-out-2"),
]
if observed != expected:
    raise SystemExit(f"expected ordered combined logs {expected}, got {observed}")
PY
ordered_logs_output="$(capture_logs_for ordered-logs-test)"
assert_contains "$ordered_logs_output" "[stdout]"
assert_contains "$ordered_logs_output" "[stderr]"
assert_contains "$ordered_logs_output" "mixed-out-1"
assert_contains "$ordered_logs_output" "mixed-err-1"
delete_ordered_logs_output="$(capture_pm2 delete ordered-logs-test)"
assert_contains "$delete_ordered_logs_output" "ordered-logs-test"

log "restart process by name"
restart_output="$(run_pm2 restart python-test)"
assert_contains "$restart_output" "python-test"
assert_contains "$restart_output" "online"
assert_line_count "$restart_output" "python-test" 1
assert_parent_is_daemon "$restart_output" "python-test"

log "restart all"
restart_all_output="$(run_pm2 restart all)"
assert_contains "$restart_all_output" "python-test"
assert_contains "$restart_all_output" "online"
assert_line_count "$restart_all_output" "python-test" 1
assert_parent_is_daemon "$restart_all_output" "python-test"

log "stop all"
stop_all_output="$(run_pm2 stop all)"
assert_contains "$stop_all_output" "python-test"
assert_contains "$stop_all_output" "stopped"

log "flush logs"
assert_file_not_empty "$stdout_log"
flush_output="$(capture_pm2 flush python-test)"
assert_contains "$flush_output" "Logs flushed"
assert_file_empty "$stdout_log"

log "start all"
start_all_output="$(run_pm2 start all)"
assert_contains "$start_all_output" "python-test"
assert_contains "$start_all_output" "online"
assert_line_count "$start_all_output" "python-test" 1
assert_parent_is_daemon "$start_all_output" "python-test"

log "dump process list"
dump_output="$(capture_pm2 dump e2e-dump)"
assert_contains "$dump_output" "Successfully saved"
[[ -s "$TMP_HOME/.pm2-go/e2e-dump.json" ]] || fail "expected dump file to exist"

log "kill daemon while process exists"
kill_output="$(capture_pm2 kill)"
assert_contains "$kill_output" "PM2 Daemon Stopped"
status_output="$(capture_pm2 status)"
assert_contains "$status_output" "PM2 Daemon Not Running"

log "restore after daemon restart"
restore_output="$(capture_pm2 restore e2e-dump)"
assert_contains "$restore_output" "Restoring processes"
restored_ls="$(run_pm2 ls)"
assert_contains "$restored_ls" "python-test"
assert_contains "$restored_ls" "online"
assert_line_count "$restored_ls" "python-test" 1
assert_parent_is_daemon "$restored_ls" "python-test"

log "delete restored process"
delete_all_output="$(capture_pm2 delete all)"
assert_contains "$delete_all_output" "python-test"
empty_ls="$(run_pm2 ls)"
assert_not_contains "$empty_ls" "python-test"

log "automatic state restore after daemon crash"
write_state_restore_ecosystem
run_pm2 start "$TMP_HOME/state-restore.json" >/dev/null
state_restore_ls="$(wait_for_ls_contains "state-restore" "online")"
assert_parent_is_daemon "$state_restore_ls" "state-restore"
[[ -s "$TMP_HOME/.pm2-go/state.json" ]] || fail "expected state file to exist"

daemon_pid="$(cat "$TMP_HOME/.pm2-go/daemon.pid")"
state_restore_pid="$(cat "$TMP_HOME/.pm2-go/pids/state-restore.pid")"
kill -9 "$daemon_pid"
wait_for_pid_exit "$daemon_pid"
kill "$state_restore_pid" >/dev/null 2>&1 || true
wait_for_pid_exit "$state_restore_pid"

state_restored_ls="$(wait_for_ls_contains "state-restore" "online")"
assert_line_count "$state_restored_ls" "state-restore" 1
assert_parent_is_daemon "$state_restored_ls" "state-restore"
run_pm2 delete state-restore >/dev/null

log "missing process errors"
assert_contains "$(capture_pm2 stop missing-process)" "not found"
assert_contains "$(capture_pm2 delete missing-process)" "not found"
assert_contains "$(capture_pm2 restart missing-process)" "not found"
assert_contains "$(capture_pm2 describe missing-process)" "Process not found"
assert_contains "$(capture_pm2 flush missing-process)" "not found"
assert_contains "$(capture_pm2 logs missing-process)" "not found"
unknown_output="$(assert_command_fails definitely-not-a-command)"
assert_contains "$unknown_output" "unknown command"

log "ecosystem environment"
write_env_ecosystem
run_pm2 start "$TMP_HOME/env.json" >/dev/null
env_log="$TMP_HOME/.pm2-go/logs/env-test-out.log"
wait_for_file_contains "$env_log" "ecosystem-value" 10
run_pm2 flush env-test >/dev/null
PM2_GO_E2E_ENV=caller-value PM2_GO_E2E_CALLER_ENV=no-update run_pm2 restart "$TMP_HOME/env.json" >/dev/null
wait_for_file_contains "$env_log" "ecosystem-value" 10
assert_file_not_contains "$env_log" "no-update"
run_pm2 flush env-test >/dev/null
PM2_GO_E2E_ENV=caller-value PM2_GO_E2E_CALLER_ENV=caller-only run_pm2 restart "$TMP_HOME/env.json" --update-env --env production >/dev/null
wait_for_file_contains "$env_log" "production-value" 10
wait_for_file_contains "$env_log" "caller-only" 10
wait_for_file_contains "$env_log" "profile-only" 10
run_pm2 delete env-test >/dev/null

log "autorestart crashed process"
write_autorestart_ecosystem
run_pm2 start "$TMP_HOME/autorestart.json" >/dev/null
wait_for_daemon_log "Restarting process autorestart-test" 15
autorestart_ls="$(run_pm2 ls)"
assert_contains "$autorestart_ls" "autorestart-test"
run_pm2 delete autorestart-test >/dev/null

log "restart limits and backoff"
write_restart_limit_ecosystem
run_pm2 start "$TMP_HOME/restart-limit.json" >/dev/null
wait_for_daemon_log "Scheduling restart for process restart-limit-test" 15
wait_for_daemon_log "Process restart-limit-test exceeded max_restarts=1" 15
restart_limit_ls="$(wait_for_ls_contains "restart-limit-test" "errored" 15)"
assert_contains "$restart_limit_ls" "restart-limit-test"
run_pm2 delete restart-limit-test >/dev/null

log "manual stop cancels delayed autorestart"
write_delayed_restart_ecosystem
run_pm2 start "$TMP_HOME/delayed-restart.json" >/dev/null
wait_for_daemon_log "Scheduling restart for process delayed-restart-test" 15
run_pm2 stop delayed-restart-test >/dev/null
sleep 3
delayed_restart_ls="$(run_pm2 ls)"
assert_contains "$delayed_restart_ls" "delayed-restart-test"
assert_contains "$delayed_restart_ls" "stopped"
assert_not_contains "$delayed_restart_ls" "online"
run_pm2 delete delayed-restart-test >/dev/null

log "memory restart"
write_memory_restart_ecosystem
run_pm2 start "$TMP_HOME/memory-restart.json" >/dev/null
memory_old_pid="$(cat "$TMP_HOME/.pm2-go/pids/memory-restart-test.pid")"
wait_for_daemon_log "Process memory-restart-test exceeded max_memory_restart=1048576 bytes" 15
memory_new_pid="$(cat "$TMP_HOME/.pm2-go/pids/memory-restart-test.pid")"
[[ "$memory_new_pid" != "$memory_old_pid" ]] || fail "expected memory restart to replace pid $memory_old_pid"
wait_for_pid_exit "$memory_old_pid" 20
memory_restart_ls="$(wait_for_ls_contains "memory-restart-test" "online" 15)"
assert_contains "$memory_restart_ls" "memory-restart-test"
run_pm2 delete memory-restart-test >/dev/null

log "health check status"
write_health_check_ecosystem
run_pm2 start "$TMP_HOME/health-check.json" >/dev/null
health_pid="$(cat "$TMP_HOME/.pm2-go/pids/health-check-test.pid")"
health_ls="$(wait_for_ls_contains "health-check-test" "unhealthy" 15)"
assert_contains "$health_ls" "health-check-test"
kill_output="$(capture_pm2 kill)"
assert_contains "$kill_output" "PM2 Daemon Stopped"
wait_for_pid_exit "$health_pid" 20

log "watch restart"
write_watch_ecosystem
run_pm2 start "$TMP_HOME/watch.json" >/dev/null
watch_ls="$(wait_for_ls_contains "watch-test" "online" 10)"
assert_contains "$watch_ls" "watch-test"
watch_old_pid="$(cat "$TMP_HOME/.pm2-go/pids/watch-test.pid")"
sleep 2
printf 'changed\n' >>"$TMP_HOME/watch-cwd/watched.txt"
wait_for_daemon_log "Watched files changed for process watch-test" 15
watch_new_pid="$(cat "$TMP_HOME/.pm2-go/pids/watch-test.pid")"
[[ "$watch_new_pid" != "$watch_old_pid" ]] || fail "expected watch restart to replace pid $watch_old_pid"
wait_for_pid_exit "$watch_old_pid" 20
watch_restarted_ls="$(wait_for_ls_contains "watch-test" "online" 15)"
assert_contains "$watch_restarted_ls" "watch-test"
run_pm2 delete watch-test >/dev/null

log "graceful reload"
mkdir -p "$TMP_HOME/reload-cwd"
cat >"$TMP_HOME/reload-cwd/reload.py" <<'PY'
import signal
import sys
import time

def shutdown(signum, frame):
    print("graceful-signal", flush=True)
    sys.exit(0)

signal.signal(signal.SIGTERM, shutdown)
print("reload-ready", flush=True)
time.sleep(20)
PY
(cd "$TMP_HOME/reload-cwd" && HOME="$TMP_HOME" "$BIN" start -- python3 reload.py >/dev/null)
reload_log="$TMP_HOME/.pm2-go/logs/python3-out.log"
wait_for_file_contains "$reload_log" "reload-ready" 10
reload_output="$(run_pm2 reload python3 --kill-timeout 2000)"
assert_contains "$reload_output" "python3"
assert_contains "$reload_output" "online"
wait_for_file_contains "$reload_log" "graceful-signal" 10
reload_ls="$(run_pm2 ls)"
assert_parent_is_daemon "$reload_ls" "python3"
run_pm2 delete python3 >/dev/null

log "direct command"
mkdir -p "$TMP_HOME/direct-cwd"
cat >"$TMP_HOME/direct-cwd/direct.py" <<'PY'
import os
import time

print(os.environ.get("PM2_GO_DIRECT_ENV"))
print(os.getcwd())
time.sleep(20)
PY
(cd "$TMP_HOME/direct-cwd" && PM2_GO_DIRECT_ENV=direct-value HOME="$TMP_HOME" "$BIN" start -- python3 direct.py >/dev/null)
direct_ls="$(wait_for_ls_contains "python3" "online")"
assert_line_count "$direct_ls" "python3" 1
assert_parent_is_daemon "$direct_ls" "python3"
direct_log="$TMP_HOME/.pm2-go/logs/python3-out.log"
wait_for_file_contains "$direct_log" "direct-value" 10
wait_for_file_contains "$direct_log" "$TMP_HOME/direct-cwd" 10
PM2_GO_DIRECT_ENV=updated-value run_pm2 restart python3 --update-env >/dev/null
wait_for_file_contains "$direct_log" "updated-value" 10
wait_for_file_contains "$direct_log" "$TMP_HOME/direct-cwd" 10

log "delete direct command"
run_pm2 delete all >/dev/null
final_ls="$(run_pm2 ls)"
assert_not_contains "$final_ls" "python3"

log "absolute executable command"
absolute_python="$(command -v python3)"
run_pm2 start -- "$absolute_python" -c 'import time; time.sleep(20)' >/dev/null
absolute_ls="$(wait_for_ls_contains "python3" "online")"
assert_line_count "$absolute_ls" "python3" 1
assert_parent_is_daemon "$absolute_ls" "python3"
[[ -f "$TMP_HOME/.pm2-go/pids/python3.pid" ]] || fail "expected sanitized pid file for absolute executable"
[[ -f "$TMP_HOME/.pm2-go/logs/python3-out.log" ]] || fail "expected sanitized stdout log for absolute executable"
run_pm2 delete python3 >/dev/null

if [[ "$E2E_SLOW" == "1" ]]; then
	log "cron restart firing"
	write_cron_ecosystem
	run_pm2 start "$TMP_HOME/cron.json" >/dev/null
	run_pm2 stop cron-test >/dev/null
	cron_ls="$(wait_for_ls_contains "cron-test" "online" 75)"
	assert_line_count "$cron_ls" "cron-test" 1
	assert_parent_is_daemon "$cron_ls" "cron-test"
	run_pm2 delete cron-test >/dev/null
else
	log "skip slow cron firing check; set E2E_SLOW=1 to enable"
fi

log "kill daemon"
run_pm2 kill >/dev/null
status_output="$(capture_pm2 status)"
assert_contains "$status_output" "PM2 Daemon Not Running"

trap - EXIT
rm -rf "$TMP_HOME"

log "e2e passed"
