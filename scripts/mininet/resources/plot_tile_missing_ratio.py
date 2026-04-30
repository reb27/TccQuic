#!/usr/bin/env python3
"""
Plot Tile Missing Ratio (%) vs Loss Rate (%).

Matrix mode (default: single LOG_DIR arg):
  Walks logs from run_article_abr_comparison.sh --matrix, groups by
  background_load_pct, writes tile_missing_ratio_bg10.png and
  tile_missing_ratio_bg25.png (one file per BG level present).

  Each figure: 1×2 subplots (BOLA | Legacy). Lines:
    FIFO — single aggregate line (no priority split)
    SP, WFQ — high (prio 0), medium (prio 1), low (prio 2)

Legacy mode (LOG_DIR + OUTPUT_PNG):
  Original SP/WFQ plot with high / medium / low for old folder layout.

CSV: statistics-*.csv columns ok, skipped, priority (see metrics.go).
Synthetic S-curves are opt-in (--synthetic-demo); by default sparse runs plot only real points.
"""

import argparse
import csv
import os
import sys
from collections import defaultdict

import numpy as np
import matplotlib.pyplot as plt

PRIORITY_MAP = {0: 'high', 1: 'medium', 2: 'low'}
PRIORITY_ORDER = ['high', 'medium', 'low']

SCHEDULER_COLORS_LEGACY = {
    'sp': '#e07b39',
    'wfq': '#3a9a5b',
}

SCHEDULER_COLORS_MATRIX = {
    'fifo': '#1f2937',
    'sp': '#e07b39',
    'wfq': '#3a9a5b',
}

SCHED_LABEL_MATRIX = {
    'fifo': 'FIFO',
    'sp': 'SP',
    'wfq': 'WFQ',
}

PRIORITY_LINESTYLES = {
    'high': '-',
    'medium': '-.',
    'low': '--',
}

PRIORITY_MARKERS = {
    'high': 'o',
    'medium': 's',
    'low': '^',
}

ABR_TITLES = {'bola': 'ABR BOLA', 'legacy': 'Legacy'}
ABR_ORDER = ['bola', 'legacy']

MATRIX_SCHEDULERS = ('fifo', 'sp', 'wfq')
MATRIX_PRIO = ('high', 'medium', 'low')


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


def _compute_missing_ratio(csv_paths: list) -> dict:
    """Return {priority_int: missing_ratio_percent}."""
    counts = defaultdict(lambda: [0, 0])
    for path in csv_paths:
        try:
            with open(path, newline='', encoding='utf-8') as f:
                for row in csv.DictReader(f):
                    prio_raw = row.get('priority')
                    if prio_raw is None or str(prio_raw).strip() == '':
                        continue
                    if _parse_bool(row.get('skipped', 'false')):
                        continue
                    try:
                        prio = int(prio_raw)
                    except (TypeError, ValueError):
                        continue
                    ok = _parse_bool(row.get('ok', 'false'))
                    counts[prio][1] += 1
                    if not ok:
                        counts[prio][0] += 1
        except Exception as exc:
            print(f'[warn] {path}: {exc}')

    result = {}
    for prio, (missing, total) in counts.items():
        result[prio] = 100.0 * missing / total if total else 0.0
    return result


def _compute_missing_overall(csv_paths: list) -> float:
    """Aggregate missing ratio over all non-skipped rows (FIFO baseline)."""
    missing = 0
    total = 0
    for path in csv_paths:
        try:
            with open(path, newline='', encoding='utf-8') as f:
                for row in csv.DictReader(f):
                    if _parse_bool(row.get('skipped', 'false')):
                        continue
                    ok = _parse_bool(row.get('ok', 'false'))
                    total += 1
                    if not ok:
                        missing += 1
        except Exception as exc:
            print(f'[warn] {path}: {exc}')
    return 100.0 * missing / total if total else 0.0


def _find_experiments(root_dir: str):
    for dirpath, _dirs, files in os.walk(root_dir):
        if 'experiment.env' in files:
            env = _parse_env(os.path.join(dirpath, 'experiment.env'))
            yield env, dirpath


def _csv_list(dirpath: str) -> list:
    return sorted(
        os.path.join(dirpath, n) for n in os.listdir(dirpath)
        if n.startswith('statistics-') and n.endswith('.csv')
        and 'summary' not in n
    )


def collect_data(root_dir: str) -> dict:
    """
    Legacy: {(abr, scheduler, priority_name, loss_pct): avg_missing_ratio}.
    """
    raw = defaultdict(list)

    for env, dirpath in _find_experiments(root_dir):
        scheduler = env.get('server_mode', '')
        abr = env.get('abr_mode', '')
        try:
            loss_pct = float(env.get('loss_pct', -1))
        except ValueError:
            continue

        if scheduler not in ('sp', 'wfq') or abr not in ('bola', 'legacy'):
            continue

        csv_files = _csv_list(dirpath)
        if not csv_files:
            continue

        ratios = _compute_missing_ratio(csv_files)
        for prio_int, ratio in ratios.items():
            pname = PRIORITY_MAP.get(prio_int, f'p{prio_int}')
            raw[(abr, scheduler, pname, loss_pct)].append(ratio)

    return {k: float(np.mean(v)) for k, v in raw.items()}


def detect_matrix_background_levels(root_dir: str) -> list:
    """Sorted unique background_load_pct for matrix-style experiments."""
    seen = set()
    for env, _dirpath in _find_experiments(root_dir):
        if env.get('scenario') != 'matrix':
            continue
        try:
            bg = int(float(env.get('background_load_pct', '')))
        except (TypeError, ValueError):
            continue
        seen.add(bg)
    return sorted(seen)


def collect_matrix_data(root_dir: str, bg_filter: int) -> dict:
    """
    {(abr, scheduler, prio_key, loss_pct): avg_ratio}
    prio_key is 'overall' for fifo, else 'high' / 'medium' / 'low'.
    """
    raw = defaultdict(list)

    for env, dirpath in _find_experiments(root_dir):
        if env.get('scenario') != 'matrix':
            continue
        scheduler = env.get('server_mode', '')
        abr = env.get('abr_mode', '')
        try:
            bg = int(float(env.get('background_load_pct', -999)))
            loss_pct = float(env.get('loss_pct', -1))
        except ValueError:
            continue
        if bg != bg_filter:
            continue
        if scheduler not in MATRIX_SCHEDULERS or abr not in ('bola', 'legacy'):
            continue

        csv_files = _csv_list(dirpath)
        if not csv_files:
            continue

        if scheduler == 'fifo':
            overall = _compute_missing_overall(csv_files)
            raw[(abr, 'fifo', 'overall', loss_pct)].append(overall)
        else:
            ratios = _compute_missing_ratio(csv_files)
            for prio_int, pname in ((0, 'high'), (1, 'medium'), (2, 'low')):
                if prio_int in ratios:
                    raw[(abr, scheduler, pname, loss_pct)].append(ratios[prio_int])

    return {k: float(np.mean(v)) for k, v in raw.items()}


def _generate_synthetic() -> dict:
    x = np.linspace(0, 25, 60)
    params = {
        ('bola', 'sp', 'high'): (0.35, 9),
        ('bola', 'sp', 'medium'): (0.40, 7),
        ('bola', 'sp', 'low'): (0.50, 4),
        ('bola', 'wfq', 'high'): (0.30, 11),
        ('bola', 'wfq', 'medium'): (0.35, 9),
        ('bola', 'wfq', 'low'): (0.45, 5),
        ('legacy', 'sp', 'high'): (0.38, 8),
        ('legacy', 'sp', 'medium'): (0.43, 6),
        ('legacy', 'sp', 'low'): (0.52, 3.5),
        ('legacy', 'wfq', 'high'): (0.33, 10),
        ('legacy', 'wfq', 'medium'): (0.38, 8),
        ('legacy', 'wfq', 'low'): (0.48, 4.5),
    }
    data = {}
    for (abr, sched, prio), (k, x0) in params.items():
        y = 100.0 / (1.0 + np.exp(-k * (x - x0)))
        for xi, yi in zip(x, y):
            data[(abr, sched, prio, float(xi))] = float(yi)
    return data


def _loss_axis_from_data(data: dict) -> tuple:
    """Returns (xmin, xmax, xticks, sorted_losses)."""
    losses = sorted({k[3] for k in data})
    if not losses:
        return 0.0, 25.0, [0, 5, 10, 15, 20, 25], losses
    lo, hi = min(losses), max(losses)
    if len(losses) == 1:
        pad = 2.0
    else:
        pad = max(0.5, 0.02 * (hi - lo))
    xmin = max(0.0, lo - pad)
    xmax = min(25.0, hi + pad)
    if len(losses) <= 10:
        xticks = losses
    else:
        xticks = list(range(0, 26, 5))
    return xmin, xmax, xticks, losses


def _generate_synthetic_matrix(bg_pct: int) -> dict:
    """S-curves shifted by background load for demo when data is sparse."""
    x = np.linspace(0, 25, 60)
    shift = 0.8 * (bg_pct / 25.0)
    data = {}
    # (abr, sched, prio_key, loss) -> y
    specs = [
        ('bola', 'fifo', 'overall', 0.32, 11.0 + shift),
        ('legacy', 'fifo', 'overall', 0.30, 10.5 + shift),
        ('bola', 'sp', 'high', 0.38, 9.5 + shift),
        ('bola', 'sp', 'medium', 0.40, 7.8 + shift),
        ('bola', 'sp', 'low', 0.42, 6.0 + shift),
        ('legacy', 'sp', 'high', 0.36, 9.0 + shift),
        ('legacy', 'sp', 'medium', 0.41, 7.2 + shift),
        ('legacy', 'sp', 'low', 0.44, 5.5 + shift),
        ('bola', 'wfq', 'high', 0.34, 10.5 + shift),
        ('bola', 'wfq', 'medium', 0.37, 8.5 + shift),
        ('bola', 'wfq', 'low', 0.40, 6.5 + shift),
        ('legacy', 'wfq', 'high', 0.33, 10.0 + shift),
        ('legacy', 'wfq', 'medium', 0.36, 8.0 + shift),
        ('legacy', 'wfq', 'low', 0.41, 6.0 + shift),
    ]
    for abr, sched, prio, k, x0 in specs:
        y = 100.0 / (1.0 + np.exp(-k * (x - x0)))
        for xi, yi in zip(x, y):
            data[(abr, sched, prio, float(xi))] = float(np.clip(yi, 0.0, 100.0))
    return data


def plot_legacy(data: dict, output_path: str):
    try:
        plt.style.use('seaborn-v0_8-whitegrid')
    except Exception:
        pass

    fig, axes = plt.subplots(1, 2, figsize=(14, 6), sharey=True)
    xmin, xmax, xticks, loss_values = _loss_axis_from_data(data)
    x_max = max(loss_values) if loss_values else 25

    for ax_idx, abr in enumerate(ABR_ORDER):
        ax = axes[ax_idx]
        abr_series = []

        for scheduler in ('sp', 'wfq'):
            colour = SCHEDULER_COLORS_LEGACY[scheduler]
            for prio in PRIORITY_ORDER:
                points = sorted(
                    (loss, ratio) for (a, s, p, loss), ratio in data.items()
                    if a == abr and s == scheduler and p == prio
                )
                if not points:
                    continue
                xs = [p[0] for p in points]
                ys = [p[1] for p in points]
                ls = PRIORITY_LINESTYLES.get(prio, '-')
                mk = PRIORITY_MARKERS.get(prio, 'o')
                label = f'{scheduler.upper()}, {prio} prio'
                abr_series.append((scheduler, prio, xs, ys))
                ax.plot(xs, ys, color=colour, linestyle=ls,
                        marker=mk, markersize=4, linewidth=1.8, alpha=0.95,
                        label=label)

        ax.set_title(ABR_TITLES.get(abr, abr), fontsize=13, fontweight='bold')
        ax.set_xlabel('Loss rate (%)', fontsize=11)
        if ax_idx == 0:
            ax.set_ylabel('Tile missing ratio (%)', fontsize=11)
        ax.set_xlim(xmin, xmax)
        ax.set_ylim(0, 100)
        if xticks:
            ax.set_xticks(xticks)
        ax.set_yticks([0, 20, 40, 60, 80, 100])
        ax.grid(axis='y', linestyle='--', alpha=0.5)
        ax.grid(axis='x', visible=False)

        if loss_values and min(loss_values) <= 5:
            _add_inset(ax, abr_series, SCHEDULER_COLORS_LEGACY, x_max)

    handles, labels = axes[0].get_legend_handles_labels()
    fig.legend(handles, labels, loc='upper center', ncol=3, fontsize=9,
               bbox_to_anchor=(0.5, 1.02))
    fig.tight_layout(rect=[0, 0, 1, 0.91])
    os.makedirs(os.path.dirname(output_path) or '.', exist_ok=True)
    fig.savefig(output_path, dpi=200, bbox_inches='tight')
    plt.close(fig)
    print(f'Saved: {output_path}')


def _add_inset(ax, series_specs, colour_map, x_max):
    """series_specs: (scheduler, prio, xs, ys); prio is 'overall' or high/medium/low."""
    if x_max <= 0:
        return
    low_loss_ys = []
    for spec in series_specs:
        xs, ys = spec[2], spec[3]
        for x, y in zip(xs, ys):
            if x <= 5:
                low_loss_ys.append(y)
    if not low_loss_ys:
        for spec in series_specs:
            low_loss_ys.extend(spec[3])
    if not low_loss_ys:
        return

    y_min = max(0.0, min(low_loss_ys) - 2.0)
    y_max = min(100.0, max(low_loss_ys) + 2.0)
    if y_max - y_min < 8.0:
        center = 0.5 * (y_min + y_max)
        y_min = max(0.0, center - 4.0)
        y_max = min(100.0, center + 4.0)

    axins = ax.inset_axes([0.45, 0.08, 0.52, 0.42])
    for spec in series_specs:
        sched, prio, xs, ys = spec[0], spec[1], spec[2], spec[3]
        colour = colour_map[sched]
        if prio == 'overall':
            ls, mk = '-', 's'
        else:
            ls = PRIORITY_LINESTYLES.get(prio, '-')
            mk = PRIORITY_MARKERS.get(prio, 'o')
        axins.plot(xs, ys, color=colour, linestyle=ls,
                   marker=mk, markersize=3, linewidth=1.4, alpha=0.95)

    axins.set_xlim(0, 5)
    axins.set_ylim(y_min, y_max)
    axins.tick_params(labelsize=7)
    axins.set_xlabel('Loss rate (%)', fontsize=7)
    axins.text(0.03, 0.95, 'zoom-in', transform=axins.transAxes,
               fontsize=8, fontstyle='italic', fontweight='bold', va='top')


def plot_matrix(data: dict, output_path: str, bg_caption: str):
    try:
        plt.style.use('seaborn-v0_8-whitegrid')
    except Exception:
        pass

    fig, axes = plt.subplots(1, 2, figsize=(14, 6), sharey=True)
    xmin, xmax, xticks, loss_values = _loss_axis_from_data(data)
    x_max = max(loss_values) if loss_values else 25

    sched_order = ['fifo', 'sp', 'wfq']

    for ax_idx, abr in enumerate(ABR_ORDER):
        ax = axes[ax_idx]
        abr_series = []

        for sched in sched_order:
            colour = SCHEDULER_COLORS_MATRIX[sched]
            if sched == 'fifo':
                points = sorted(
                    (loss, ratio) for (a, s, p, loss), ratio in data.items()
                    if a == abr and s == 'fifo' and p == 'overall'
                )
                if not points:
                    continue
                xs = [p[0] for p in points]
                ys = [p[1] for p in points]
                abr_series.append((sched, 'overall', xs, ys))
                ax.plot(xs, ys, color=colour, linestyle='-', marker='s',
                        markersize=4, linewidth=2.0, alpha=0.95,
                        label=SCHED_LABEL_MATRIX[sched])
            else:
                for prio in MATRIX_PRIO:
                    points = sorted(
                        (loss, ratio) for (a, s, p, loss), ratio in data.items()
                        if a == abr and s == sched and p == prio
                    )
                    if not points:
                        continue
                    xs = [p[0] for p in points]
                    ys = [p[1] for p in points]
                    abr_series.append((sched, prio, xs, ys))
                    ls = PRIORITY_LINESTYLES[prio]
                    mk = PRIORITY_MARKERS[prio]
                    label = f'{SCHED_LABEL_MATRIX[sched]}, {prio} prio'
                    ax.plot(xs, ys, color=colour, linestyle=ls,
                            marker=mk, markersize=4, linewidth=1.8, alpha=0.95,
                            label=label)

        ax.set_title(ABR_TITLES.get(abr, abr), fontsize=13, fontweight='bold')
        ax.set_xlabel('Loss rate (%)', fontsize=11)
        if ax_idx == 0:
            ax.set_ylabel('Tile missing ratio (%)', fontsize=11)
        ax.set_xlim(xmin, xmax)
        ax.set_ylim(0, 100)
        if xticks:
            ax.set_xticks(xticks)
        ax.set_yticks([0, 20, 40, 60, 80, 100])
        ax.grid(axis='y', linestyle='--', alpha=0.5)
        ax.grid(axis='x', visible=False)
        if loss_values and min(loss_values) <= 5:
            _add_inset(ax, abr_series, SCHEDULER_COLORS_MATRIX, x_max)

    fig.suptitle(
        f'Tile missing ratio vs loss ({bg_caption} background traffic)',
        fontsize=12, fontweight='bold', y=1.02,
    )
    handles, labels = axes[0].get_legend_handles_labels()
    fig.legend(handles, labels, loc='upper center', ncol=4, fontsize=7,
               bbox_to_anchor=(0.5, 0.99))
    fig.tight_layout(rect=[0, 0, 1, 0.86])
    os.makedirs(os.path.dirname(output_path) or '.', exist_ok=True)
    fig.savefig(output_path, dpi=200, bbox_inches='tight')
    plt.close(fig)
    print(f'Saved: {output_path}')


def main():
    parser = argparse.ArgumentParser(
        description='Plot tile missing ratio vs packet loss (matrix or legacy log trees).',
    )
    parser.add_argument(
        'log_dir',
        help='Root directory containing experiment.env subfolders',
    )
    parser.add_argument(
        'output_png',
        nargs='?',
        help='Optional: single output path (legacy layout mode)',
    )
    parser.add_argument(
        '--synthetic-demo',
        action='store_true',
        help='If fewer than 3 loss points, plot fake S-curves (old default; not real data).',
    )
    args = parser.parse_args()
    root_dir = args.log_dir
    use_synthetic = args.synthetic_demo

    if not os.path.isdir(root_dir):
        print(f'Error: {root_dir} is not a directory')
        sys.exit(1)

    if args.output_png:
        output_path = args.output_png
        data = collect_data(root_dir)
        distinct_losses = {loss for (_, _, _, loss) in data}
        if len(distinct_losses) < 3 and use_synthetic:
            print('[info] Sparse legacy data — using synthetic demonstration curves (--synthetic-demo).')
            data = _generate_synthetic()
        elif len(distinct_losses) < 3:
            print('[info] Sparse legacy data — plotting real points only (use --synthetic-demo for fake curves).')
        plot_legacy(data, output_path)
        return

    bgs = detect_matrix_background_levels(root_dir)
    if not bgs:
        print('[warn] No scenario=matrix experiments found; '
              'writing single tile_missing_ratio.png with legacy collector.')
        data = collect_data(root_dir)
        distinct_losses = {loss for (_, _, _, loss) in data}
        if len(distinct_losses) < 3 and use_synthetic:
            data = _generate_synthetic()
        elif len(distinct_losses) < 3:
            print('[info] Sparse data — plotting real points only.')
        out = os.path.join(root_dir, 'tile_missing_ratio.png')
        plot_legacy(data, out)
        return

    for bg in bgs:
        data = collect_matrix_data(root_dir, bg)
        distinct_losses = {loss for (_, _, _, loss) in data}
        if len(distinct_losses) < 3 and use_synthetic:
            print(f'[info] BG {bg}%: only {len(distinct_losses)} loss point(s) '
                  f'— synthetic demo curves (--synthetic-demo).')
            data = _generate_synthetic_matrix(bg)
        elif len(distinct_losses) < 3:
            print(
                f'[info] BG {bg}%: only {len(distinct_losses)} loss point(s) '
                f'— plotting real data only.',
            )
        out = os.path.join(root_dir, f'tile_missing_ratio_bg{bg}.png')
        plot_matrix(data, out, f'{bg}%')


if __name__ == '__main__':
    main()
