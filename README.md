# PM2-GO

[![CI](https://github.com/dunstorm/pm2-go/actions/workflows/ci.yml/badge.svg)](https://github.com/dunstorm/pm2-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dunstorm/pm2-go)](https://goreportcard.com/report/github.com/dunstorm/pm2-go)

PM2-GO is a small, PM2-inspired process manager written in Go. It runs a local
daemon, starts managed applications from that daemon, and exposes a CLI for the
usual process lifecycle work: start, list, restart, stop, delete, logs, dump,
and restore.

This project is a work in progress. It is not a full drop-in replacement for
PM2. Linux and macOS are supported; Windows is not currently supported.

## Features

- Start direct commands or JSON ecosystem files
- Keep process metadata in a local daemon
- List and describe managed processes
- Stop, restart, delete, and flush logs by name, id, JSON file, or `all`
- Tail stdout and stderr logs
- Auto-restart crashed processes
- Limit crash-loop restarts with `max_restarts`, `min_uptime`, and restart delays
- Restart processes that exceed `max_memory_restart`
- Restart processes on cron schedules
- Restore online processes automatically when the daemon starts
- Dump and restore process lists
- Rotate logs by size and file count

## Requirements

- Go 1.24 or newer
- Linux or macOS
- Docker, only if you want to run the containerized end-to-end checks

## Install

Install from the module:

```sh
go install github.com/dunstorm/pm2-go/cmd/pm2-go@latest
```

Or build from a local checkout:

```sh
git clone https://github.com/dunstorm/pm2-go.git
cd pm2-go
make build
./bin/pm2-go --version
```

To install the checked-out version into your Go binary path:

```sh
make install
```

## Quick Start

From the repository root:

```sh
pm2-go start python3 examples/test.py
pm2-go ls
pm2-go describe python3
pm2-go logs -l 50 python3
pm2-go restart python3
pm2-go stop python3
pm2-go delete python3
pm2-go kill
```

Starting a process starts the daemon automatically if it is not already running.
Managed applications are spawned by the daemon, and the CLI talks to the daemon
over local gRPC.

## Ecosystem Files

PM2-GO can start multiple processes from a JSON file. The file can be a raw
array or an object with an `apps` array.

```json
[
  {
    "name": "python-test",
    "args": ["test.py"],
    "autorestart": true,
    "cwd": "./examples",
    "env": {
      "APP_ENV": "production"
    },
    "env_staging": {
      "APP_ENV": "staging"
    },
    "executable_path": "python3",
    "cron_restart": "* * * * *"
  }
]
```

Run it with:

```sh
pm2-go start examples/ecosystem.json
pm2-go start examples/ecosystem.json --env staging
pm2-go restart examples/ecosystem.json
pm2-go stop examples/ecosystem.json
pm2-go delete examples/ecosystem.json
```

Supported fields:

| Field | Description |
| --- | --- |
| `name` | Stable process name used by CLI commands and log files. |
| `executable_path` | Command or executable path to run. |
| `args` | Arguments passed to the executable. |
| `cwd` | Working directory for the process. |
| `env` | Environment variables added to the spawned process. |
| `env_<name>` | Environment profile selected with `--env <name>`; profile values override `env`. |
| `autorestart` | Restart the process when it exits unexpectedly. |
| `cron_restart` | Five-field cron expression for scheduled restarts. |
| `max_restarts` | Maximum unstable restarts before the process is marked `errored`. |
| `min_uptime` | Minimum stable runtime in milliseconds before crash counters reset. |
| `restart_delay` | Fixed autorestart delay in milliseconds. |
| `exp_backoff_restart_delay` | Initial autorestart delay in milliseconds, doubled after each unstable crash. |
| `max_memory_restart` | RSS limit in bytes before PM2-GO restarts the process. |

PM2-GO stores a complete environment with process metadata for restart, dump,
and daemon restore flows. Direct commands use the shell environment from
`pm2-go start`. JSON ecosystem files use the shell environment as a base and
let the file's `env` values override matching keys. Keep `$HOME/.pm2-go`
private if you store sensitive values there.

To update a running process with the current shell environment, restart with
`--update-env`:

```sh
APP_ENV=production pm2-go restart api --update-env
```

For JSON ecosystem files, `--update-env` refreshes that shell environment base
and still lets the file's `env` values override matching keys.

## Commands

| Command | Purpose |
| --- | --- |
| `pm2-go start <cmd> [args...]` | Start a direct command. |
| `pm2-go start [--env name] <file.json>` | Start or restart processes from an ecosystem file. |
| `pm2-go start all` | Start all known processes. |
| `pm2-go ls [--json]` | List managed processes. |
| `pm2-go describe [--json] <name\|id>` | Show process details and log paths. |
| `pm2-go logs [-l lines] <name\|id>` | Tail stdout and stderr logs. |
| `pm2-go stop <name\|id\|file.json\|all>` | Stop processes without removing them from the process list. |
| `pm2-go restart [--env name] [--update-env] <name\|id\|file.json\|all>` | Restart processes. |
| `pm2-go reload [--env name] [--signal SIGTERM] [--kill-timeout 1600] [--update-env] <name\|id\|file.json\|all>` | Gracefully reload processes before force-kill fallback. |
| `pm2-go delete <name\|id\|file.json\|all>` | Stop and remove processes from the process list. |
| `pm2-go flush [name\|id\|file.json\|all]` | Truncate process log files. |
| `pm2-go dump [name]` | Save the current process list to `$HOME/.pm2-go/<name>.json`. |
| `pm2-go restore [name]` | Restore a dumped process list. |
| `pm2-go config` | Print local PM2-GO config. |
| `pm2-go status [--json]` | Show daemon status. |
| `pm2-go kill` | Stop the daemon and managed processes. |

## Daemon and Files

PM2-GO stores runtime data under `$HOME/.pm2-go` by default. Set
`PM2_GO_HOME` to use a different runtime directory for isolated tests or
side-by-side migrations.

| Path | Purpose |
| --- | --- |
| `daemon.pid` | Current daemon PID. |
| `pids/` | Managed process PID files. |
| `logs/` | Process stdout and stderr logs. |
| `config.json` | Local PM2-GO configuration. |
| `state.json` | Automatically persisted online processes restored on daemon start. |
| `*.json` | Dump files created by `pm2-go dump`. |

You can start the daemon explicitly with:

```sh
pm2-go -d
```

## Logs and Rotation

Each process writes to two files:

- `$HOME/.pm2-go/logs/<name>-out.log`
- `$HOME/.pm2-go/logs/<name>-err.log`

Enable log rotation:

```sh
pm2-go config set logrotate true
pm2-go config set logrotate_size 10M
pm2-go config set logrotate_max_files 10
```

Defaults are `logrotate=false`, `logrotate_size=10M`, and
`logrotate_max_files=10`.

## Benchmarking

PM2-GO includes a small benchmark harness that compares PM2 and PM2-GO CLI
lifecycle latency. It measures command elapsed time for cold start, list,
restart, stop, start, and delete flows with managed Python processes.

Run it in Docker, which installs PM2 without changing the host:

```sh
make benchmark/docker
```

If `pm2` is already installed locally, run it directly:

```sh
make benchmark
```

The benchmark accepts custom sample counts and process counts:

```sh
./scripts/benchmark_pm2_vs_pm2_go.py --iterations 10 --process-counts 1,10,50
```

Example median results from the Docker benchmark on Linux arm64 with 5
iterations:

| Processes | Scenario | PM2 | PM2-GO | Result |
| --- | --- | ---: | ---: | --- |
| 1 | cold start | 293.4ms | 17.2ms | PM2-GO 17.07x faster |
| 1 | list | 79.1ms | 2.7ms | PM2-GO 29.53x faster |
| 1 | restart all | 185.8ms | 4.2ms | PM2-GO 43.92x faster |
| 10 | cold start | 322.0ms | 51.8ms | PM2-GO 6.22x faster |
| 10 | list | 70.5ms | 8.8ms | PM2-GO 7.98x faster |
| 10 | restart all | 646.3ms | 31.3ms | PM2-GO 20.64x faster |

These numbers measure CLI lifecycle overhead only. They do not measure app
throughput, long-running daemon memory use, or behavior under production load.

## Development

Build the CLI:

```sh
make build
```

Run unit tests:

```sh
make test
```

Run end-to-end CLI checks locally:

```sh
make test/e2e
```

Run the same end-to-end checks inside a lightweight Docker image:

```sh
make test/e2e/docker
```

Run the slower cron-firing e2e scenario:

```sh
make test/e2e/slow
make test/e2e/docker/slow
```

Run the Go vulnerability scanner:

```sh
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Regenerate protobuf code after changing `proto/process.proto`:

```sh
make protoc
```

## Releases

GoReleaser configuration lives in `.goreleaser.yaml`. It builds static Linux and
macOS binaries from `./cmd/pm2-go`.

For a local release dry run:

```sh
goreleaser release --snapshot --clean
```

Publishing a release is tag-driven. Push a version tag from `main`:

```sh
git tag v0.1.2
git push origin v0.1.2
```

The `Release` GitHub Actions workflow runs GoReleaser and publishes the GitHub
release from that tag.
