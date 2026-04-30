#!/usr/bin/env python3
"""
Simple ABR comparison: BOLA vs Legacy only (WFQ, overall tile missing ratio).

Walks LOG_ROOT for experiment.env with scenario=matrix, server_mode=wfq,
abr_mode in {bola, legacy}. Groups by loss_pct and background_load_pct.

Default output: LOG_ROOT/tile_missing_abr_simple_bg<BG>.png

Does not require 3+ loss points (unlike plot_tile_missing_ratio fallback).
"""
from __future__ import annotations

import csv
import os
import sys
from collections import defaultdict


def _parse_bool(v: str) -> bool:
    return str(v).strip().lower() in ('true', '1', 'yes')


def _parse_env(path: str) -> dict:
    env = {}
    with open(path, encoding='utf-8-sig') as f:
        for line in f:
            line = line.strip()
            if '=' in line and not line.startswith('#'):
                k, v = line.split('=', 1)
                env[k.strip()] = v.strip()
    return env


def _csv_list(dirpath: str) -> list:
    return sorted(
        os.path.join(dirpath, n)
        for n in os.listdir(dirpath)
        if n.startswith('statistics-') and n.endswith('.csv') and 'summary' not in n
    )


def missing_ratio_overall(csv_paths: list) -> float:
    miss = tot = 0
    for path in csv_paths:
        try:
            with open(path, newline='', encoding='utf-8') as f:
                for row in csv.DictReader(f):
                    if _parse_bool(row.get('skipped', 'false')):
                        continue
                    tot += 1
                    if not _parse_bool(row.get('ok', 'false')):
                        miss += 1
        except OSError as exc:
            print(f'[warn] {path}: {exc}')
    return 100.0 * miss / tot if tot else 0.0


def collect_wfq_bola_legacy(root: str):
    """
    Returns:
      by_bg: bg -> loss -> abr -> overall missing ratio (%)
    """
    by_bg = defaultdict(lambda: defaultdict(dict))

    for dirpath, _, files in os.walk(root):
        if 'experiment.env' not in files:
            continue
        env = _parse_env(os.path.join(dirpath, 'experiment.env'))
        if env.get('scenario') != 'matrix':
            continue
        if env.get('server_mode') != 'wfq':
            continue
        abr = env.get('abr_mode', '')
        if abr not in ('bola', 'legacy'):
            continue
        try:
            bg = int(float(env.get('background_load_pct', -1)))
            loss = float(env.get('loss_pct', -1))
        except ValueError:
            continue
        if bg < 0 or loss < 0:
            continue
        csvs = _csv_list(dirpath)
        if not csvs:
            print(f'[warn] no statistics CSV in {dirpath}')
            continue
        overall = missing_ratio_overall(csvs)
        by_bg[bg][loss][abr] = overall

    return by_bg


def _fmt_loss_list(losses: list) -> str:
    return ', '.join(f'{x:g}%' for x in losses)


def plot_one_bg_simple(bg: int, loss_abr: dict, out_path: str):
    import matplotlib.pyplot as plt

    losses = sorted(loss_abr.keys())
    if not losses:
        print(f'[warn] BG {bg}%: no data')
        return

    bola_pts = sorted(lo for lo in losses if 'bola' in loss_abr[lo])
    leg_pts = sorted(lo for lo in losses if 'legacy' in loss_abr[lo])
    only_bola = sorted(lo for lo in losses if 'bola' in loss_abr[lo] and 'legacy' not in loss_abr[lo])
    only_legacy = sorted(lo for lo in losses if 'legacy' in loss_abr[lo] and 'bola' not in loss_abr[lo])
    both = sorted(lo for lo in losses if 'bola' in loss_abr[lo] and 'legacy' in loss_abr[lo])

    fig, ax = plt.subplots(figsize=(7.5, 5.2))

    for abr, color, marker in (
        ('bola', '#1d4ed8', 'o'),
        ('legacy', '#b45309', 's'),
    ):
        xs, ys = [], []
        for loss in losses:
            if abr not in loss_abr[loss]:
                continue
            xs.append(loss)
            ys.append(loss_abr[loss][abr])
        if xs:
            label = 'ABR BOLA' if abr == 'bola' else 'Legacy'
            ax.plot(xs, ys, color=color, marker=marker, markersize=8, linewidth=2.2, label=label)

    ax.set_xlabel('Loss rate (%)', fontsize=11)
    ax.set_ylabel('Tile missing ratio (%)', fontsize=11)
    ax.set_title(
        f'Tile missing vs loss — BOLA vs Legacy (WFQ, {bg}% background)',
        fontsize=12,
        fontweight='bold',
    )
    ymax = 0.0
    for lo in losses:
        for abr in ('bola', 'legacy'):
            if abr in loss_abr[lo]:
                ymax = max(ymax, float(loss_abr[lo][abr]))
    ax.set_ylim(0, min(100.0, max(5.0, ymax * 1.15 + 1.0)))
    ax.set_xticks(losses)
    ax.grid(axis='y', linestyle='--', alpha=0.45)
    ax.legend(loc='best', fontsize=10)

    note_lines = [
        f'Cobertura no log: BOLA em [{_fmt_loss_list(bola_pts)}] · Legacy em [{_fmt_loss_list(leg_pts)}].',
        'Cada linha só une pontos onde existe statistics-*.csv na pasta da corrida.',
    ]
    if only_bola:
        note_lines.append(
            f'Atenção: em {_fmt_loss_list(only_bola)} só há BOLA — não dá para comparar Legacy nesses pontos.'
        )
    if only_legacy:
        note_lines.append(
            f'Atenção: em {_fmt_loss_list(only_legacy)} só há Legacy — não dá para comparar BOLA nesses pontos.'
        )
    if both:
        note_lines.append(f'Comparação direta possível em: {_fmt_loss_list(both)}.')

    fig.text(
        0.5,
        0.01,
        '\n'.join(note_lines),
        ha='center',
        va='bottom',
        fontsize=8.5,
        color='#374151',
        linespacing=1.35,
    )

    fig.subplots_adjust(bottom=0.28)
    os.makedirs(os.path.dirname(out_path) or '.', exist_ok=True)
    fig.savefig(out_path, dpi=160, bbox_inches='tight')
    plt.close(fig)
    print(f'Saved: {out_path}')
    for line in note_lines:
        print(f'[info] {line}')


def main():
    if len(sys.argv) < 2:
        print(f'Usage: {sys.argv[0]} <LOG_ROOT> [OUTPUT_PNG]')
        sys.exit(1)
    root = sys.argv[1]
    if not os.path.isdir(root):
        print(f'Not a directory: {root}')
        sys.exit(1)

    by_bg = collect_wfq_bola_legacy(root)
    if not by_bg:
        print('No matrix WFQ bola/legacy experiments found.')
        sys.exit(2)

    if len(sys.argv) >= 3:
        bg = sorted(by_bg.keys())[0]
        plot_one_bg_simple(bg, by_bg[bg], sys.argv[2])
        return

    for bg in sorted(by_bg.keys()):
        out = os.path.join(root, f'tile_missing_abr_simple_bg{bg}.png')
        plot_one_bg_simple(bg, by_bg[bg], out)


if __name__ == '__main__':
    main()
