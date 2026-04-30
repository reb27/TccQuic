#!/usr/bin/env python3
"""
End-to-end: start server, run test-client with a short segment window, stop server,
then write bitrate_e2e.png from the new statistics-*.csv.

Requires: Go 1.19.x on PATH (quic-go v0.31 in this repo does not build on Go 1.20+),
dataset under data/segments, matplotlib.

Usage (from repo root):
  python scripts/run_e2e_bitrate_proof.py
  python scripts/run_e2e_bitrate_proof.py --policy wfq --segments 10 --out results/bitrate_e2e.png

Only plot an existing CSV:
  python scripts/run_e2e_bitrate_proof.py --csv-only statistics-12345.csv --out out.png
"""
from __future__ import annotations

import argparse
import glob
import os
import re
import subprocess
import sys
import time


def repo_root() -> str:
    return os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))


def newest_statistics_csv(root: str, since_unix: float) -> str | None:
    paths = glob.glob(os.path.join(root, "statistics-*.csv"))
    paths = [p for p in paths if "summary" not in os.path.basename(p).lower()]
    if not paths:
        return None
    # Prefer files created/updated during this run (avoids picking an old CSV).
    fresh = [p for p in paths if os.path.getmtime(p) >= since_unix - 2.0]
    pool = fresh if fresh else paths
    pool.sort(key=lambda p: os.path.getmtime(p), reverse=True)
    return pool[0]


def go_version_supported() -> tuple[bool, str]:
    try:
        out = subprocess.check_output(["go", "version"], text=True, stderr=subprocess.STDOUT)
    except (OSError, subprocess.CalledProcessError) as e:
        return False, str(e)
    m = re.search(r"go(\d+)\.(\d+)", out)
    if not m:
        return True, out.strip()
    major, minor = int(m.group(1)), int(m.group(2))
    if major > 1 or (major == 1 and minor > 19):
        return False, out.strip()
    return True, out.strip()


def kill_process_tree(pid: int) -> None:
    if pid <= 0:
        return
    if os.name == "nt":
        subprocess.run(
            ["taskkill", "/F", "/T", "/PID", str(pid)],
            capture_output=True,
            text=True,
        )
    else:
        try:
            os.killpg(os.getpgid(pid), signal.SIGTERM)
        except (ProcessLookupError, PermissionError, OSError):
            try:
                os.kill(pid, signal.SIGTERM)
            except OSError:
                pass


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--policy", default="fifo", help="Server queue policy (fifo, sp, wfq, ...)")
    ap.add_argument("--segments", type=int, default=12, help="TEST_CLIENT_SEGMENT_LIMIT")
    ap.add_argument("--parallel", type=int, default=48, help="test-client parallelism")
    ap.add_argument("--latency-ms", type=int, default=800, help="test-client base latency ms")
    ap.add_argument("--host", default="localhost")
    ap.add_argument("--port", type=int, default=8000)
    ap.add_argument("--out", default="", help="Output PNG (default: results/bitrate_e2e_<ts>.png)")
    ap.add_argument("--server-wait", type=float, default=4.0, help="Seconds to wait after starting server")
    ap.add_argument("--csv-only", default="", help="Skip run; only plot this statistics CSV")
    ap.add_argument(
        "--ignore-go-version",
        action="store_true",
        help="Do not abort when Go is newer than 1.19 (build may still fail)",
    )
    args = ap.parse_args()

    root = repo_root()
    plot_script = os.path.join(root, "scripts", "plot_bitrate_e2e.py")

    if args.csv_only:
        csv_path = args.csv_only if os.path.isabs(args.csv_only) else os.path.join(root, args.csv_only)
        if not os.path.isfile(csv_path):
            sys.exit(f"CSV not found: {csv_path}")
    else:
        csv_path = ""
        ok_go, go_line = go_version_supported()
        if not ok_go and not args.ignore_go_version:
            sys.stderr.write(
                "Go version is not supported for this repo's quic-go (need Go 1.19.x).\n"
                f"  Detected: {go_line}\n"
                "  Install Go 1.19, or pass --ignore-go-version if you know the build works.\n"
            )
            sys.exit(2)
        if not ok_go:
            sys.stderr.write(f"WARNING: unsupported Go; continuing anyway. ({go_line})\n")

    out = args.out
    if not out:
        os.makedirs(os.path.join(root, "results"), exist_ok=True)
        out = os.path.join(root, "results", f"bitrate_e2e_{int(time.time())}.png")
    elif not os.path.isabs(out):
        out = os.path.join(root, out)

    if args.csv_only:
        subprocess.check_call([sys.executable, plot_script, csv_path, out], cwd=root)
        print("Done (plot only):", out)
        return

    env = os.environ.copy()
    env["TEST_CLIENT_SEGMENT_LIMIT"] = str(max(1, args.segments))

    run_started = time.time()

    go = ["go", "run", "main.go", "server", args.policy]
    print("Starting server:", " ".join(go), flush=True)
    pop_kw: dict = {
        "cwd": root,
        "stdin": subprocess.DEVNULL,
        "stdout": subprocess.DEVNULL,
        "stderr": subprocess.DEVNULL,
    }
    if os.name == "nt":
        pop_kw["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP  # type: ignore[attr-defined]
    else:
        pop_kw["preexec_fn"] = os.setsid  # type: ignore[attr-defined]
    srv = subprocess.Popen(go, **pop_kw)
    time.sleep(max(0.5, args.server_wait))

    cli = [
        "go",
        "run",
        "main.go",
        "test-client",
        args.host,
        str(args.parallel),
        str(args.latency_ms),
    ]
    print("Running client:", " ".join(cli), flush=True)
    rc = 1
    try:
        rc = subprocess.call(cli, cwd=root, env=env)
    finally:
        print("Stopping server pid", srv.pid)
        if os.name == "nt":
            kill_process_tree(srv.pid)
        else:
            kill_process_tree(srv.pid)
        try:
            srv.wait(timeout=5)
        except subprocess.TimeoutExpired:
            srv.kill()

    if rc != 0:
        sys.stderr.write(f"test-client exit code {rc}\n")

    csv_new = newest_statistics_csv(root, run_started)
    if not csv_new:
        sys.exit("No statistics-*.csv found after run.")

    print("Using statistics file:", csv_new)
    subprocess.check_call([sys.executable, plot_script, csv_new, out], cwd=root)
    print("End-to-end complete. Chart:", out)
    if rc != 0:
        sys.exit(rc)


if __name__ == "__main__":
    main()
