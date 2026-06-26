#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import math
import os
import platform
import shutil
import socket
import statistics
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[1]
PM2_GO_PORT = 50051


@dataclass
class Sample:
    tool: str
    process_count: int
    scenario: str
    iteration: int
    elapsed_ms: float


class CommandError(RuntimeError):
    pass


class Runner:
    def __init__(self, name: str, binary: Path, home: Path, cwd: Path, timeout: float) -> None:
        self.name = name
        self.binary = binary
        self.home = home
        self.cwd = cwd
        self.timeout = timeout

    def env(self) -> dict[str, str]:
        env = os.environ.copy()
        env["NO_COLOR"] = "1"
        env["CLICOLOR"] = "0"
        if self.name == "pm2":
            env["PM2_HOME"] = str(self.home)
        else:
            env["PM2_GO_HOME"] = str(self.home)
            env["HOME"] = str(self.home.parent / "home")
        return env

    def command(self, *args: str) -> list[str]:
        return [str(self.binary), *args]

    def run(self, args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
        completed = subprocess.run(
            self.command(*args),
            cwd=self.cwd,
            env=self.env(),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=self.timeout,
        )
        if check and completed.returncode != 0:
            raise CommandError(
                "\n".join(
                    [
                        f"{self.name} command failed: {' '.join(self.command(*args))}",
                        f"exit code: {completed.returncode}",
                        "stdout:",
                        completed.stdout.strip(),
                        "stderr:",
                        completed.stderr.strip(),
                    ]
                )
            )
        return completed

    def timed(self, args: list[str]) -> float:
        start = time.perf_counter()
        self.run(args)
        return (time.perf_counter() - start) * 1000

    def start_config_args(self, config: Path) -> list[str]:
        return ["start", str(config)]

    def list_args(self) -> list[str]:
        return ["ls"]

    def restart_all_args(self) -> list[str]:
        return ["restart", "all"]

    def stop_all_args(self) -> list[str]:
        return ["stop", "all"]

    def start_all_args(self) -> list[str]:
        return ["start", "all"]

    def delete_all_args(self) -> list[str]:
        return ["delete", "all"]

    def kill_args(self) -> list[str]:
        return ["kill"]

    def reset_home(self) -> None:
        self.run(self.kill_args(), check=False)
        time.sleep(0.2)
        shutil.rmtree(self.home, ignore_errors=True)
        self.home.mkdir(parents=True, exist_ok=True)


class PM2Runner(Runner):
    pass


class PM2GoRunner(Runner):
    def reset_home(self) -> None:
        self.run(self.kill_args(), check=False)
        wait_for_port_closed(PM2_GO_PORT)
        shutil.rmtree(self.home, ignore_errors=True)
        self.home.mkdir(parents=True, exist_ok=True)


def parse_process_counts(value: str) -> list[int]:
    counts: list[int] = []
    for raw in value.split(","):
        raw = raw.strip()
        if not raw:
            continue
        count = int(raw)
        if count < 1:
            raise argparse.ArgumentTypeError("process counts must be positive")
        counts.append(count)
    if not counts:
        raise argparse.ArgumentTypeError("at least one process count is required")
    return counts


def build_pm2_go(output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["go", "build", "-o", str(output), "./cmd/pm2-go"],
        cwd=ROOT_DIR,
        check=True,
    )


def is_port_open(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.settimeout(0.2)
        return sock.connect_ex(("127.0.0.1", port)) == 0


def wait_for_port_closed(port: int, timeout: float = 5) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if not is_port_open(port):
            return
        time.sleep(0.1)
    raise CommandError(
        f"127.0.0.1:{port} is still in use. Stop the existing pm2-go daemon before running local benchmarks."
    )


def write_app_script(path: Path) -> None:
    path.write_text(
        "\n".join(
            [
                "import signal",
                "import sys",
                "import time",
                "",
                "def stop(signum, frame):",
                "    sys.exit(0)",
                "",
                "signal.signal(signal.SIGINT, stop)",
                "signal.signal(signal.SIGTERM, stop)",
                "",
                "while True:",
                "    time.sleep(1)",
                "",
            ]
        ),
        encoding="utf-8",
    )


def write_pm2_go_config(path: Path, python_bin: str, app_script: Path, count: int, cwd: Path) -> None:
    apps = [
        {
            "name": f"bench-{index}",
            "executable_path": python_bin,
            "args": [str(app_script), str(index)],
            "cwd": str(cwd),
            "autorestart": False,
            "env": {
                "PYTHONUNBUFFERED": "1",
            },
        }
        for index in range(count)
    ]
    path.write_text(json.dumps(apps, indent=2) + "\n", encoding="utf-8")


def js_string(value: str) -> str:
    return json.dumps(value)


def write_pm2_config(path: Path, python_bin: str, app_script: Path, count: int, cwd: Path, log_dir: Path) -> None:
    apps: list[str] = []
    for index in range(count):
        name = f"bench-{index}"
        apps.append(
            "\n".join(
                [
                    "    {",
                    f"      name: {js_string(name)},",
                    f"      script: {js_string(python_bin)},",
                    f"      args: [{js_string(str(app_script))}, {js_string(str(index))}],",
                    "      interpreter: 'none',",
                    "      autorestart: false,",
                    f"      cwd: {js_string(str(cwd))},",
                    f"      out_file: {js_string(str(log_dir / (name + '-out.log')))},",
                    f"      error_file: {js_string(str(log_dir / (name + '-err.log')))},",
                    "      env: { PYTHONUNBUFFERED: '1' }",
                    "    }",
                ]
            )
        )
    path.write_text(
        "module.exports = {\n  apps: [\n"
        + ",\n".join(apps)
        + "\n  ]\n};\n",
        encoding="utf-8",
    )


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0
    ordered = sorted(values)
    index = max(0, math.ceil((pct / 100) * len(ordered)) - 1)
    return ordered[index]


def summarize(values: list[float]) -> dict[str, float]:
    return {
        "samples": float(len(values)),
        "min_ms": min(values),
        "median_ms": statistics.median(values),
        "mean_ms": statistics.fmean(values),
        "p95_ms": percentile(values, 95),
        "max_ms": max(values),
    }


def record(samples: list[Sample], runner: Runner, process_count: int, scenario: str, iteration: int, elapsed_ms: float) -> None:
    samples.append(
        Sample(
            tool=runner.name,
            process_count=process_count,
            scenario=scenario,
            iteration=iteration,
            elapsed_ms=elapsed_ms,
        )
    )


def bench_cold_start(
    samples: list[Sample],
    runner: Runner,
    process_count: int,
    config: Path,
    iterations: int,
) -> None:
    for iteration in range(1, iterations + 1):
        runner.reset_home()
        record(samples, runner, process_count, "cold_start", iteration, runner.timed(runner.start_config_args(config)))
        runner.run(runner.kill_args(), check=False)


def bench_running_commands(
    samples: list[Sample],
    runner: Runner,
    process_count: int,
    config: Path,
    iterations: int,
) -> None:
    runner.reset_home()
    runner.run(runner.start_config_args(config))

    for iteration in range(1, iterations + 1):
        record(samples, runner, process_count, "ls", iteration, runner.timed(runner.list_args()))

    for iteration in range(1, iterations + 1):
        record(samples, runner, process_count, "restart_all", iteration, runner.timed(runner.restart_all_args()))

    for iteration in range(1, iterations + 1):
        record(samples, runner, process_count, "stop_all", iteration, runner.timed(runner.stop_all_args()))
        record(samples, runner, process_count, "start_all", iteration, runner.timed(runner.start_all_args()))

    runner.run(runner.kill_args(), check=False)


def bench_delete_all(
    samples: list[Sample],
    runner: Runner,
    process_count: int,
    config: Path,
    iterations: int,
) -> None:
    for iteration in range(1, iterations + 1):
        runner.reset_home()
        runner.run(runner.start_config_args(config))
        record(samples, runner, process_count, "delete_all", iteration, runner.timed(runner.delete_all_args()))
        runner.run(runner.kill_args(), check=False)


def print_environment(pm2_bin: Path, pm2_go_bin: Path, iterations: int, process_counts: list[int]) -> None:
    print("Benchmark environment")
    print(f"  platform: {platform.platform()}")
    print(f"  python: {sys.version.split()[0]}")
    print(f"  iterations: {iterations}")
    print(f"  process counts: {', '.join(str(count) for count in process_counts)}")
    print(f"  pm2: {pm2_bin}")
    print(f"  pm2-go: {pm2_go_bin}")
    print()
    print("Times are CLI command elapsed milliseconds. Lower is better.")
    print()


def print_table(samples: list[Sample]) -> None:
    grouped: dict[tuple[str, int, str], list[float]] = {}
    for sample in samples:
        grouped.setdefault((sample.tool, sample.process_count, sample.scenario), []).append(sample.elapsed_ms)

    rows: list[tuple[str, int, str, dict[str, float]]] = []
    for key, values in grouped.items():
        rows.append((*key, summarize(values)))

    rows.sort(key=lambda item: (item[1], item[2], item[0]))

    print(f"{'tool':<8} {'procs':>5} {'scenario':<14} {'median':>10} {'mean':>10} {'p95':>10} {'min':>10} {'max':>10}")
    print("-" * 86)
    for tool, count, scenario, stats in rows:
        print(
            f"{tool:<8} {count:>5} {scenario:<14} "
            f"{stats['median_ms']:>10.1f} {stats['mean_ms']:>10.1f} {stats['p95_ms']:>10.1f} "
            f"{stats['min_ms']:>10.1f} {stats['max_ms']:>10.1f}"
        )

    print()
    print("Median comparison")
    print("-" * 86)
    by_scenario: dict[tuple[int, str], dict[str, float]] = {}
    for tool, count, scenario, stats in rows:
        by_scenario.setdefault((count, scenario), {})[tool] = stats["median_ms"]

    for (count, scenario), values in sorted(by_scenario.items()):
        if "pm2" not in values or "pm2-go" not in values:
            continue
        pm2 = values["pm2"]
        pm2_go = values["pm2-go"]
        if pm2_go == 0:
            continue
        ratio = pm2 / pm2_go
        if ratio >= 1:
            verdict = f"pm2-go faster by {ratio:.2f}x"
        else:
            verdict = f"pm2 faster by {(1 / ratio):.2f}x"
        print(f"{count:>2} process(es) {scenario:<14} {verdict}")


def write_outputs(samples: list[Sample], json_path: Path | None, csv_path: Path | None) -> None:
    if json_path:
        json_path.parent.mkdir(parents=True, exist_ok=True)
        json_path.write_text(
            json.dumps([sample.__dict__ for sample in samples], indent=2) + "\n",
            encoding="utf-8",
        )

    if csv_path:
        csv_path.parent.mkdir(parents=True, exist_ok=True)
        with csv_path.open("w", encoding="utf-8", newline="") as handle:
            writer = csv.DictWriter(handle, fieldnames=["tool", "process_count", "scenario", "iteration", "elapsed_ms"])
            writer.writeheader()
            for sample in samples:
                writer.writerow(sample.__dict__)


def main() -> int:
    parser = argparse.ArgumentParser(description="Compare PM2 and PM2-GO CLI lifecycle latency.")
    parser.add_argument("--iterations", type=int, default=5, help="Samples per scenario.")
    parser.add_argument("--process-counts", type=parse_process_counts, default=parse_process_counts("1,10"))
    parser.add_argument("--pm2-bin", type=Path, default=None, help="Path to the pm2 binary.")
    parser.add_argument("--pm2-go-bin", type=Path, default=None, help="Path to an existing pm2-go binary.")
    parser.add_argument("--json", type=Path, default=None, help="Write raw samples to a JSON file.")
    parser.add_argument("--csv", type=Path, default=None, help="Write raw samples to a CSV file.")
    parser.add_argument("--keep-temp", action="store_true", help="Keep temporary benchmark files.")
    parser.add_argument("--timeout", type=float, default=30, help="Command timeout in seconds.")
    args = parser.parse_args()

    if args.iterations < 1:
        parser.error("--iterations must be positive")

    pm2_bin = args.pm2_bin or shutil.which("pm2")
    if pm2_bin is None:
        parser.error("pm2 was not found. Install it or run `make benchmark/docker`.")
    pm2_bin = Path(pm2_bin)

    temp_dir = Path(tempfile.mkdtemp(prefix="pm2-go-benchmark-"))
    try:
        pm2_go_bin = args.pm2_go_bin or (temp_dir / "bin" / "pm2-go")
        pm2_go_bin = Path(pm2_go_bin)
        if args.pm2_go_bin is None:
            build_pm2_go(pm2_go_bin)

        python_bin = shutil.which("python3")
        if python_bin is None:
            parser.error("python3 was not found")

        workspace = temp_dir / "workspace"
        workspace.mkdir(parents=True, exist_ok=True)
        log_dir = temp_dir / "pm2-logs"
        log_dir.mkdir(parents=True, exist_ok=True)

        app_script = workspace / "bench_app.py"
        write_app_script(app_script)

        configs: dict[str, dict[int, Path]] = {"pm2": {}, "pm2-go": {}}
        for count in args.process_counts:
            pm2_config = workspace / f"pm2-{count}.config.js"
            pm2_go_config = workspace / f"pm2-go-{count}.json"
            write_pm2_config(pm2_config, python_bin, app_script, count, workspace, log_dir)
            write_pm2_go_config(pm2_go_config, python_bin, app_script, count, workspace)
            configs["pm2"][count] = pm2_config
            configs["pm2-go"][count] = pm2_go_config

        runners: list[Runner] = [
            PM2Runner("pm2", pm2_bin, temp_dir / "pm2-home", ROOT_DIR, args.timeout),
            PM2GoRunner("pm2-go", pm2_go_bin, temp_dir / "pm2-go-home", ROOT_DIR, args.timeout),
        ]

        print_environment(pm2_bin, pm2_go_bin, args.iterations, args.process_counts)

        samples: list[Sample] = []
        for count in args.process_counts:
            for runner in runners:
                config = configs[runner.name][count]
                bench_cold_start(samples, runner, count, config, args.iterations)
                bench_running_commands(samples, runner, count, config, args.iterations)
                bench_delete_all(samples, runner, count, config, args.iterations)
                runner.run(runner.kill_args(), check=False)

        print_table(samples)
        write_outputs(samples, args.json, args.csv)
        return 0
    finally:
        if args.keep_temp:
            print()
            print(f"Kept temporary files at {temp_dir}")
        else:
            shutil.rmtree(temp_dir, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())
