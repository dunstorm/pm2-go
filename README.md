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
    "executable_path": "python3",
    "cron_restart": "* * * * *"
  }
]
```

Run it with:

```sh
pm2-go start examples/ecosystem.json
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
| `autorestart` | Restart the process when it exits unexpectedly. |
| `cron_restart` | Five-field cron expression for scheduled restarts. |

Environment values are stored with process metadata for restart, dump, and
daemon restore flows. Keep `$HOME/.pm2-go` private if you store sensitive
values there.

## Commands

| Command | Purpose |
| --- | --- |
| `pm2-go start <cmd> [args...]` | Start a direct command. |
| `pm2-go start <file.json>` | Start or restart processes from an ecosystem file. |
| `pm2-go start all` | Start all known processes. |
| `pm2-go ls` | List managed processes. |
| `pm2-go describe <name\|id>` | Show process details and log paths. |
| `pm2-go logs [-l lines] <name\|id>` | Tail stdout and stderr logs. |
| `pm2-go stop <name\|id\|file.json\|all>` | Stop processes without removing them from the process list. |
| `pm2-go restart <name\|id\|file.json\|all>` | Restart processes. |
| `pm2-go delete <name\|id\|file.json\|all>` | Stop and remove processes from the process list. |
| `pm2-go flush [name\|id\|file.json\|all]` | Truncate process log files. |
| `pm2-go dump [name]` | Save the current process list to `$HOME/.pm2-go/<name>.json`. |
| `pm2-go restore [name]` | Restore a dumped process list. |
| `pm2-go config` | Print local PM2-GO config. |
| `pm2-go status` | Show daemon status. |
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
