#!/usr/bin/env python3
"""
Build a two-panel figure from a single statistics-*.csv (test client after a run).

Left: requested bitrate counts split by in_fov (end-to-end client behaviour).
Right: time vs requested bitrate (scatter), coloured by FoV.

Bitrate column: Go model enums 3=LOW, 5=MEDIUM, 10=HIGH (maps to rep 5/10/15 on server).

Usage:
  python scripts/plot_bitrate_e2e.py statistics-12345.csv out/bitrate_e2e.png
"""
from __future__ import annotations

import argparse
import csv
import os
import sys

BITRATE_LABEL = {3: "LOW (3)", 5: "MEDIUM (5)", 10: "HIGH (10)"}


def _parse_bool(v: str) -> bool:
    return str(v).strip().lower() in ("true", "1", "yes")


def load_rows(path: str):
    with open(path, newline="", encoding="utf-8", errors="replace") as f:
        r = csv.DictReader(f)
        if not r.fieldnames or "bitrate" not in r.fieldnames:
            sys.exit(f"Missing bitrate column. Header: {r.fieldnames}")
        rows = []
        for row in r:
            if _parse_bool(row.get("skipped", "false")):
                continue
            try:
                br = int(float(row["bitrate"]))
            except (TypeError, ValueError):
                continue
            rows.append(
                {
                    "time_ns": int(float(row.get("time_ns", 0) or 0)),
                    "bitrate": br,
                    "in_fov": _parse_bool(row.get("in_fov", "false")),
                    "ok": _parse_bool(row.get("ok", "false")),
                }
            )
    return rows


def plot(rows: list, title_suffix: str, out_path: str) -> None:
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        sys.exit("No non-skipped rows to plot.")

    t0 = min(r["time_ns"] for r in rows)
    t_s = np.array([(r["time_ns"] - t0) / 1e9 for r in rows], dtype=float)
    br = np.array([r["bitrate"] for r in rows], dtype=int)
    fov = np.array([r["in_fov"] for r in rows], dtype=bool)
    ok = np.array([r["ok"] for r in rows], dtype=bool)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(12.5, 5.0))

    # --- Left: grouped counts (FoV vs non-FoV) x bitrate ---
    bitrates = [b for b in (3, 5, 10) if np.any(br == b)]
    if not bitrates:
        bitrates = sorted(set(br.tolist()))

    x = np.arange(len(bitrates))
    w = 0.35
    fov_counts = [int(np.sum((br == b) & fov)) for b in bitrates]
    nfov_counts = [int(np.sum((br == b) & ~fov)) for b in bitrates]

    ax1.bar(x - w / 2, fov_counts, width=w, label="in_fov=true", color="#15803d", edgecolor="#111827", linewidth=0.5)
    ax1.bar(x + w / 2, nfov_counts, width=w, label="in_fov=false", color="#4b5563", edgecolor="#111827", linewidth=0.5)
    ax1.set_xticks(x)
    ax1.set_xticklabels([BITRATE_LABEL.get(b, str(b)) for b in bitrates])
    ax1.set_ylabel("Request count (non-skipped)")
    ax1.set_title("Requested bitrate by FoV (client log)")
    ax1.legend()
    ax1.grid(axis="y", linestyle="--", alpha=0.45)

    # --- Right: timeline ---
    jitter = (np.random.RandomState(42).rand(len(br)) - 0.5) * 0.35
    colors = np.where(fov, "#15803d", "#6b7280")
    ax2.scatter(t_s, br.astype(float) + jitter, c=colors, s=8, alpha=0.65, linewidths=0)
    ax2.set_xlabel("Elapsed time (s) from first logged request")
    ax2.set_ylabel("Bitrate enum (+ jitter)")
    ax2.set_yticks([3, 5, 10])
    ax2.set_yticklabels([BITRATE_LABEL.get(3), BITRATE_LABEL.get(5), BITRATE_LABEL.get(10)])
    ax2.set_title("Timeline (green=FoV, grey=non-FoV)")
    ax2.grid(True, linestyle="--", alpha=0.35)

    ok_n = int(np.sum(ok))
    fig.suptitle(f"End-to-end bitrate differentiation {title_suffix}\nok={ok_n}/{len(rows)} non-skipped rows", fontsize=12, fontweight="bold")
    fig.subplots_adjust(top=0.82, wspace=0.28)
    os.makedirs(os.path.dirname(os.path.abspath(out_path)) or ".", exist_ok=True)
    fig.savefig(out_path, dpi=160, bbox_inches="tight")
    plt.close(fig)
    print("Saved:", out_path)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("statistics_csv", help="statistics-<pid>.csv from repo root after test-client")
    ap.add_argument("output_png", help="Output image path")
    args = ap.parse_args()
    if not os.path.isfile(args.statistics_csv):
        sys.exit(f"File not found: {args.statistics_csv}")
    rows = load_rows(args.statistics_csv)
    base = os.path.basename(args.statistics_csv)
    plot(rows, f"({base})", args.output_png)


if __name__ == "__main__":
    main()
