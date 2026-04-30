#!/usr/bin/env python3
"""
Plot client-requested bitrate mix from statistics-*.csv (after bitrate column exists).

Expects ABR enum values in column bitrate: 3=LOW (rep 5), 5=MEDIUM (rep 10), 10=HIGH (rep 15).

Usage:
  python3 plot_bitrate_from_statistics.py <LOG_ROOT> [OUTPUT_PNG]

LOG_ROOT: directory tree with experiment.env + statistics-*.csv (walks recursively).
If OUTPUT_PNG omitted, writes <LOG_ROOT>/bitrate_mix.png
"""
from __future__ import annotations

import csv
import os
import sys
from collections import defaultdict


def _parse_bool(v: str) -> bool:
    return str(v).strip().lower() in ('true', '1', 'yes')


BITRATE_LABEL = {
    3: 'LOW (rep 5)',
    5: 'MEDIUM (rep 10)',
    10: 'HIGH (rep 15)',
}


def _csv_paths(root: str) -> list:
    out = []
    for dirpath, _, files in os.walk(root):
        for n in files:
            if n.startswith('statistics-') and n.endswith('.csv') and 'summary' not in n:
                out.append(os.path.join(dirpath, n))
    return sorted(out)


def aggregate_bitrates(csv_paths: list) -> tuple[dict, int, int]:
    """Returns (counts_by_bitrate_int, total_with_bitrate, rows_missing_bitrate)."""
    counts: dict = defaultdict(int)
    total = 0
    missing_col = 0
    for path in csv_paths:
        try:
            with open(path, newline='', encoding='utf-8') as f:
                for row in csv.DictReader(f):
                    if _parse_bool(row.get('skipped', 'false')):
                        continue
                    br = row.get('bitrate')
                    if br is None or str(br).strip() == '':
                        missing_col += 1
                        continue
                    try:
                        b = int(float(br))
                    except (TypeError, ValueError):
                        missing_col += 1
                        continue
                    total += 1
                    counts[b] += 1
        except OSError as exc:
            print(f'[warn] {path}: {exc}')
    if missing_col:
        print(f'[warn] {missing_col} non-skipped rows without bitrate (old client CSV?)')
    return dict(counts), total, missing_col


def plot_bar(counts: dict, total: int, missing_bitrate_rows: int, title: str, out_path: str):
    import matplotlib.pyplot as plt

    order = [3, 5, 10]
    extras = sorted(k for k in counts if k not in order)
    keys = [k for k in order if counts.get(k, 0) > 0] + extras
    values = [counts[k] for k in keys]
    if not keys or total == 0:
        print('[warn] No bitrate data to plot.')
        return

    pct = [100.0 * v / total for v in values]

    fig, ax = plt.subplots(figsize=(7.5, 4.8))
    colors = ['#1d4ed8', '#ca8a04', '#15803d']
    bar_cols = [
        colors[order.index(k)] if k in order else '#6b7280'
        for k in keys
    ]

    x = range(len(keys))
    bars = ax.bar(x, pct, color=bar_cols, edgecolor='#111827', linewidth=0.6)
    ax.set_xticks(list(x))
    ax.set_xticklabels(
        [BITRATE_LABEL.get(k, f'raw={k}') for k in keys],
        rotation=15,
        ha='right',
    )
    ax.set_ylabel('Share of non-skipped requests (%)', fontsize=11)
    ax.set_title(title, fontsize=12, fontweight='bold')
    ax.set_ylim(0, min(100.0, max(pct) * 1.15 + 2.0))
    ax.grid(axis='y', linestyle='--', alpha=0.45)

    for bar, v, n in zip(bars, pct, values):
        ax.text(
            bar.get_x() + bar.get_width() / 2,
            bar.get_height() + 0.6,
            f'{v:.1f}%\n(n={n})',
            ha='center',
            va='bottom',
            fontsize=9,
            color='#374151',
        )

    note = (
        f'Rows with bitrate: {total}. '
        + (f'Excluded (no bitrate column): {missing_bitrate_rows}. ' if missing_bitrate_rows else '')
        + 'Server maps 3→rep5, 5→rep10, 10→rep15.'
    )
    fig.text(0.5, 0.02, note, ha='center', fontsize=8.5, color='#4b5563')
    fig.subplots_adjust(bottom=0.22)
    os.makedirs(os.path.dirname(out_path) or '.', exist_ok=True)
    fig.savefig(out_path, dpi=160, bbox_inches='tight')
    plt.close(fig)
    print(f'Saved: {out_path}')


def main():
    if len(sys.argv) < 2:
        print(f'Usage: {sys.argv[0]} <LOG_ROOT> [OUTPUT_PNG]')
        sys.exit(1)
    root = sys.argv[1]
    out = sys.argv[2] if len(sys.argv) >= 3 else os.path.join(root, 'bitrate_mix.png')
    if not os.path.isdir(root):
        print(f'Not a directory: {root}')
        sys.exit(1)

    paths = _csv_paths(root)
    if not paths:
        print('No statistics-*.csv found under', root)
        sys.exit(2)

    counts, total, miss = aggregate_bitrates(paths)
    title = 'Requested bitrate mix (client statistics)'
    plot_bar(counts, total, miss, title, out)


if __name__ == '__main__':
    main()
