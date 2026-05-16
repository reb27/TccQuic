#!/usr/bin/env python3
"""
Gráfico agrupado: mistura de bitrate (LOW/MEDIUM/HIGH) por cenário Mininet.

Espera um diretório “super log” com subpastas (uma por corrida), cada uma com
`experiment.env` e `statistics-*.csv` (com coluna `bitrate`).

Uso:
  python3 plot_bola_bitrate_scenarios.py <SUPER_LOG_DIR> [OUTPUT_PNG]

Se OUTPUT_PNG for omitido, grava <SUPER_LOG_DIR>/bola_bitrate_por_cenario.png

Gráfico de demonstração (sem Mininet, dados sintéticos → results/bola_bitrate_demo.png):
  python3 plot_bola_bitrate_scenarios.py --demo
"""
from __future__ import annotations

import csv
import os
import shutil
import sys
from collections import defaultdict
from pathlib import Path


BITRATE_ORDER = (3, 5, 10)
BITRATE_LABEL = {
    3: 'LOW (rep 5)',
    5: 'MEDIUM (rep 10)',
    10: 'HIGH (rep 15)',
}


def _parse_bool(v: str) -> bool:
    return str(v).strip().lower() in ('true', '1', 'yes')


def _read_env(path: str) -> dict:
    out: dict = {}
    try:
        with open(path, encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith('#') or '=' not in line:
                    continue
                k, _, v = line.partition('=')
                out[k.strip()] = v.strip()
    except OSError:
        pass
    return out


def _scenario_label(env: dict, dirname: str) -> str:
    if env.get('plot_label'):
        return env['plot_label']
    cbw = env.get('client_bw_mbps', '')
    if cbw:
        return f'Cliente {cbw} Mbps'
    return env.get('scenario', dirname)


def _aggregate_csvs_in_dir(dirpath: str) -> tuple[dict, int]:
    counts: dict = defaultdict(int)
    total = 0
    for name in os.listdir(dirpath):
        if not name.startswith('statistics-') or not name.endswith('.csv'):
            continue
        if 'summary' in name.lower():
            continue
        path = os.path.join(dirpath, name)
        try:
            with open(path, newline='', encoding='utf-8') as f:
                for row in csv.DictReader(f):
                    if _parse_bool(row.get('skipped', 'false')):
                        continue
                    br = row.get('bitrate')
                    if br is None or str(br).strip() == '':
                        continue
                    try:
                        b = int(float(br))
                    except (TypeError, ValueError):
                        continue
                    total += 1
                    counts[b] += 1
        except OSError as exc:
            print(f'[warn] {path}: {exc}')
    return dict(counts), total


def _discover_runs(root: str) -> list[tuple[str, str, dict, dict, int]]:
    """Lista (dirpath, dirname, env, counts, total) com total > 0."""
    runs = []
    for name in sorted(os.listdir(root)):
        dirpath = os.path.join(root, name)
        if not os.path.isdir(dirpath):
            continue
        env_path = os.path.join(dirpath, 'experiment.env')
        if not os.path.isfile(env_path):
            continue
        env = _read_env(env_path)
        counts, total = _aggregate_csvs_in_dir(dirpath)
        if total == 0:
            print(f'[warn] Sem linhas com bitrate em {dirpath} — ignorado.')
            continue
        runs.append((dirpath, name, env, counts, total))
    return runs


_STATS_HEADER = (
    'time_ns,segment,tile,priority,latency_ns,timedout,skipped,ok,'
    'tp,buffer_s,tile_missing_ratio,in_fov,on_time,bitrate\n'
)


def _demo_write_superlog(root: Path) -> None:
    """Três cenários fictícios (banda baixa → alta) com misturas de bitrate distintas."""
    if root.exists():
        shutil.rmtree(root)
    scenarios: list[tuple[str, str, list[tuple[int, int]]]] = [
        ('cbw030', '30', [(3, 70), (5, 22), (10, 8)]),
        ('cbw080', '80', [(3, 35), (5, 40), (10, 25)]),
        ('cbw200', '200', [(3, 12), (5, 28), (10, 60)]),
    ]
    row_line = '1,1,1,1,1,false,false,true,1,1,0,true,true,{}\n'
    for tag, cbw, mix in scenarios:
        d = root / tag
        d.mkdir(parents=True)
        (d / 'experiment.env').write_text(
            f'scenario={tag}\nplot_label=Cliente {cbw} Mbps\nclient_bw_mbps={cbw}\n',
            encoding='utf-8',
        )
        lines = [_STATS_HEADER]
        for br, n in mix:
            lines.extend(row_line.format(br) for _ in range(n))
        (d / 'statistics-demo.csv').write_text(''.join(lines), encoding='utf-8')


def plot_grouped(runs: list, out_path: str, *, title: str | None = None) -> None:
    import matplotlib.pyplot as plt
    import numpy as np

    if not runs:
        print('Nenhuma corrida com dados de bitrate encontrada.')
        sys.exit(2)

    labels = [_scenario_label(r[2], r[1]) for r in runs]
    n = len(runs)
    x = np.arange(n, dtype=float)
    width = 0.24
    fig, ax = plt.subplots(figsize=(max(7.0, 1.2 * n + 3), 5.2))

    colors = {'3': '#1d4ed8', '5': '#ca8a04', '10': '#15803d'}
    for i, br in enumerate(BITRATE_ORDER):
        heights = []
        for r in runs:
            _, _, _, counts, total = r
            heights.append(100.0 * counts.get(br, 0) / total if total else 0.0)
        offset = (i - 1) * width
        ax.bar(
            x + offset,
            heights,
            width,
            label=BITRATE_LABEL.get(br, str(br)),
            color=colors.get(str(br), '#6b7280'),
            edgecolor='#111827',
            linewidth=0.5,
        )

    ax.set_xticks(x)
    ax.set_xticklabels(labels, rotation=18, ha='right')
    ax.set_ylabel('Parte dos pedidos não ignorados (%)', fontsize=11)
    ax.set_title(
        title or 'BOLA (Mininet): mistura de bitrate pedida por cenário',
        fontsize=12,
        fontweight='bold',
    )
    ax.set_ylim(0, 100)
    ax.grid(axis='y', linestyle='--', alpha=0.45)
    ax.legend(loc='upper right', fontsize=9)
    fig.subplots_adjust(bottom=0.22)
    os.makedirs(os.path.dirname(out_path) or '.', exist_ok=True)
    fig.savefig(out_path, dpi=160, bbox_inches='tight')
    plt.close(fig)
    print(f'Saved: {out_path}')


def main() -> None:
    if len(sys.argv) >= 2 and sys.argv[1] == '--demo':
        repo_root = Path(__file__).resolve().parents[3]
        results = repo_root / 'results'
        demo_root = results / '_demo_bola_superlog'
        out_png = results / 'bola_bitrate_demo.png'
        _demo_write_superlog(demo_root)
        runs = _discover_runs(str(demo_root))
        plot_grouped(
            runs,
            str(out_png),
            title='BOLA: mistura de bitrate por cenário (demo sintético)',
        )
        print(f'Abrir: {out_png}')
        return

    if len(sys.argv) < 2:
        print(f'Usage: {sys.argv[0]} <SUPER_LOG_DIR> [OUTPUT_PNG] | --demo')
        sys.exit(1)
    root = sys.argv[1]
    out = (
        sys.argv[2]
        if len(sys.argv) >= 3
        else os.path.join(root, 'bola_bitrate_por_cenario.png')
    )
    if not os.path.isdir(root):
        print(f'Not a directory: {root}')
        sys.exit(1)

    runs = _discover_runs(root)
    plot_grouped(runs, out)


if __name__ == '__main__':
    main()
