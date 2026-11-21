#!/usr/bin/env python3
"""Measure the wall time and peak memory of `lafke` on pinned public corpora.

Corpora are cloned at the exact revision in corpora.json, so every run analyzes the same
source. Each measurement runs `lafke` in a fresh process. Results are written as JSON
together with the environment they were measured in.
"""
import argparse
import json
import os
import platform
import statistics
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def run(cmd, **kw):
    return subprocess.run(cmd, check=True, text=True, capture_output=True, **kw).stdout.strip()


def fetch(corpus, cache):
    dest = cache / corpus["name"]
    if not dest.exists():
        run(["git", "init", "-q", str(dest)])
        run(["git", "-C", str(dest), "remote", "add", "origin", corpus["repo"]])
    run(["git", "-C", str(dest), "fetch", "-q", "--depth", "1", "origin", corpus["rev"]])
    run(["git", "-C", str(dest), "checkout", "-q", "--detach", "FETCH_HEAD"])
    head = run(["git", "-C", str(dest), "rev-parse", "HEAD"])
    if head != corpus["rev"]:
        sys.exit(f"{corpus['name']}: expected {corpus['rev']}, got {head}")
    return dest


def measure(cmd, env):
    """Run cmd once and return (wall seconds, peak RSS in MiB, exit code)."""
    start = time.perf_counter()
    proc = subprocess.Popen(cmd, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    _, status, usage = os.wait4(proc.pid, 0)
    wall = time.perf_counter() - start
    rss = usage.ru_maxrss / (1024 * 1024 if sys.platform == "darwin" else 1024)
    return wall, rss, os.waitstatus_to_exitcode(status)


def summarize(values):
    return {
        "min": min(values),
        "median": statistics.median(values),
        "mean": statistics.fmean(values),
        "stdev": statistics.stdev(values) if len(values) > 1 else 0.0,
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--lafke", default="lafke", help="lafke executable (default: lafke on PATH)")
    ap.add_argument("--runs", type=int, default=5, help="measured runs per corpus (default 5)")
    ap.add_argument("--cache", default=str(ROOT / ".cache"), help="where corpora are cloned")
    ap.add_argument("--out", default=str(ROOT / "results"), help="directory for the JSON result")
    ap.add_argument("--only", action="append", default=[], help="run only this corpus (repeatable)")
    args = ap.parse_args()

    cache = Path(args.cache)
    cache.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    version = run([args.lafke, "version"], env=env)
    engine = run([args.lafke, "engine", "status"], env=env)

    result = {
        "date": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "environment": {
            "os": platform.platform(),
            "machine": platform.machine(),
            "cpus": os.cpu_count(),
            "python": platform.python_version(),
            "lafke": version,
            "engine": engine,
        },
        "runs_per_corpus": args.runs,
        "corpora": [],
    }

    for corpus in json.loads((ROOT / "corpora.json").read_text())["corpora"]:
        if args.only and corpus["name"] not in args.only:
            continue
        src = fetch(corpus, cache)
        target = str(src / corpus["path"])
        cmd = [args.lafke, *corpus["command"], target, "--format", "json", "--fail-on", "critical"]
        print(f"{corpus['name']}: warm-up", file=sys.stderr)
        measure(cmd, env)
        walls, rsss = [], []
        for i in range(args.runs):
            wall, rss, code = measure(cmd, env)
            if code not in (0, 1):  # 1 means findings; anything else is a failure
                sys.exit(f"{corpus['name']}: lafke exited with {code}")
            walls.append(wall)
            rsss.append(rss)
            print(f"{corpus['name']}: run {i + 1}/{args.runs} {wall:.2f}s {rss:.0f} MiB", file=sys.stderr)
        result["corpora"].append({
            "name": corpus["name"],
            "repo": corpus["repo"],
            "rev": corpus["rev"],
            "command": [args.lafke, *corpus["command"]],
            "wall_seconds": summarize(walls),
            "peak_rss_mib": summarize(rsss),
            "samples": {"wall_seconds": walls, "peak_rss_mib": rsss},
        })

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    path = out / f"{datetime.now(timezone.utc):%Y%m%dT%H%M%SZ}.json"
    path.write_text(json.dumps(result, indent=2) + "\n")
    print(path)


if __name__ == "__main__":
    main()
