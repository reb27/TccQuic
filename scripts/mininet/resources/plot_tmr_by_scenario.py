#!/usr/bin/env python3
"""
Lê os resultados da matriz experimental e gera scatter plot:
  - 4 subplots: Cenário #1, #2, #5, #6
  - Eixo X: política (FIFO, SP, WFQ)
  - Eixo Y: TMR (%)
  - Laranja = FoV (high priority)
  - Azul = non-FoV (low priority)
  - Cada ponto = 1 repetição

Uso:
  python plot_tmr_by_scenario.py <MATRIX_DIR> [--abr bola|legacy] [--clients 1|6] [--mix balanced|wide_heavy] [-o output.png]

Exemplo:
  python plot_tmr_by_scenario.py ../../logs/matrix-001 --abr bola --clients 6 --mix balanced
"""

from __future__ import annotations

import argparse
import csv
import os
import sys
from collections import defaultdict
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np

# ─── Constantes de estilo (alinhadas ao artigo) ────────────────────────────────

POLICY_ORDER  = ("fifo", "sp", "wfq")
POLICY_LABELS = {"fifo": "FIFO", "sp": "SP", "wfq": "WFQ"}
POLICY_COLORS = {"fifo": "#1f77b4", "sp": "#ff7f0e", "wfq": "#2ca02c"}  # azul/laranja/verde

SCENARIO_ORDER = ("1", "2", "5", "6")
SCENARIO_LABELS = {
    "1": "Scenario #1\n(24ms, 10% bg)",
    "2": "Scenario #2\n(24ms, 30% bg)",
    "5": "Scenario #5\n(10ms, 10% bg)",
    "6": "Scenario #6\n(10ms, 30% bg)",
}

# Cores dos pontos por prioridade (igual ao artigo)
COLOR_FOV    = "#ff7f0e"  # laranja — high priority (FoV)
COLOR_NONFOV = "#1f77b4"  # azul    — low priority (non-FoV)

MARKER_FOV    = "o"
MARKER_NONFOV = "^"
JITTER        = 0.08  # deslocamento horizontal para separar pontos sobrepostos


# ─── Leitura dos dados ─────────────────────────────────────────────────────────

def parse_env(path: str) -> dict:
    env = {}
    with open(path, encoding="utf-8-sig") as f:
        for line in f:
            line = line.strip()
            if "=" in line and not line.startswith("#"):
                k, v = line.split("=", 1)
                env[k.strip()] = v.strip()
    return env


def find_summary_csvs(run_dir: str) -> list[str]:
    """Encontra todos os statistics-summary-*.csv de um run (1 por cliente)."""
    return sorted(
        str(p) for p in Path(run_dir).glob("statistics-summary-*.csv")
    )


def read_tmr_from_summary(csv_path: str) -> tuple[float, float] | None:
    """
    Lê deadline_miss_rate_fov_percent e deadline_miss_rate_nonfov_percent
    do summary do cliente. Retorna (tmr_fov, tmr_nonfov) ou None se vazio.
    """
    try:
        with open(csv_path, newline="", encoding="utf-8") as f:
            rows = list(csv.DictReader(f))
        if not rows:
            return None
        row = rows[-1]  # última linha (sessão completa)
        tmr_fov    = float(row.get("deadline_miss_rate_fov_percent", "nan"))
        tmr_nonfov = float(row.get("deadline_miss_rate_nonfov_percent", "nan"))
        return tmr_fov, tmr_nonfov
    except (OSError, ValueError, KeyError):
        return None


def collect(matrix_dir: str, abr: str, clients: str, mix: str) -> dict:
    """
    Retorna:
      data[scenario_id][policy] = {
          "fov":    [tmr_fov_rep1, tmr_fov_rep2, ...],   # % por repetição
          "nonfov": [tmr_nonfov_rep1, ...]
      }
    """
    data: dict = defaultdict(lambda: defaultdict(lambda: {"fov": [], "nonfov": []}))

    for dirpath, _, files in os.walk(matrix_dir):
        if "experiment.env" not in files:
            continue
        env = parse_env(os.path.join(dirpath, "experiment.env"))

        sc   = env.get("scenario_id", "").strip()
        pol  = env.get("policy", "").strip().lower()
        abr_ = env.get("abr_mode", env.get("abr", "")).strip().lower()
        nc   = env.get("num_clients", "1").strip()
        mx   = env.get("fov_mix", "balanced").strip().lower()

        if sc not in SCENARIO_ORDER:
            continue
        if pol not in POLICY_ORDER:
            continue
        if abr_ != abr.lower():
            continue
        if nc != str(clients):
            continue
        if mx != mix.lower():
            continue

        # Agrega TMR de todos os clientes desta repetição
        summaries = find_summary_csvs(dirpath)
        if not summaries:
            continue

        fov_vals, nonfov_vals = [], []
        for s in summaries:
            result = read_tmr_from_summary(s)
            if result is not None:
                fov_vals.append(result[0])
                nonfov_vals.append(result[1])

        if fov_vals:
            data[sc][pol]["fov"].append(float(np.mean(fov_vals)))
        if nonfov_vals:
            data[sc][pol]["nonfov"].append(float(np.mean(nonfov_vals)))

    return data


# ─── Plot ─────────────────────────────────────────────────────────────────────

def plot(data: dict, abr: str, clients: str, mix: str, out_path: str) -> None:
    n_scenarios = len(SCENARIO_ORDER)
    fig, axes = plt.subplots(1, n_scenarios, figsize=(3.5 * n_scenarios, 4.5), sharey=True)

    # Legenda global
    legend_handles = [
        plt.Line2D([0], [0], marker=MARKER_FOV,    color="w",
                   markerfacecolor=COLOR_FOV,    markersize=8, label="high priority (FoV)"),
        plt.Line2D([0], [0], marker=MARKER_NONFOV, color="w",
                   markerfacecolor=COLOR_NONFOV, markersize=8, label="low priority (non-FoV)"),
    ]
    fig.legend(handles=legend_handles, loc="upper center", ncol=2,
               frameon=True, fontsize=9, bbox_to_anchor=(0.5, 1.02))

    x_positions = {pol: i for i, pol in enumerate(POLICY_ORDER)}

    for ax, sc_id in zip(axes, SCENARIO_ORDER):
        sc_data = data.get(sc_id, {})

        for pol in POLICY_ORDER:
            pol_data = sc_data.get(pol, {"fov": [], "nonfov": []})
            xc = x_positions[pol]

            fov_vals    = pol_data["fov"]
            nonfov_vals = pol_data["nonfov"]

            n = max(len(fov_vals), len(nonfov_vals), 1)
            jitter_vals = np.linspace(-JITTER, JITTER, n)

            if fov_vals:
                ax.scatter(
                    [xc - 0.12 + j for j in np.linspace(-JITTER, JITTER, len(fov_vals))],
                    fov_vals,
                    color=COLOR_FOV, marker=MARKER_FOV, s=30, zorder=3, alpha=0.85
                )
            if nonfov_vals:
                ax.scatter(
                    [xc + 0.12 + j for j in np.linspace(-JITTER, JITTER, len(nonfov_vals))],
                    nonfov_vals,
                    color=COLOR_NONFOV, marker=MARKER_NONFOV, s=30, zorder=3, alpha=0.85
                )

        ax.set_title(SCENARIO_LABELS[sc_id], fontsize=9)
        ax.set_xticks(list(x_positions.values()))
        ax.set_xticklabels([POLICY_LABELS[p] for p in POLICY_ORDER], fontsize=9)
        ax.set_xlim(-0.6, len(POLICY_ORDER) - 0.4)
        ax.yaxis.set_major_locator(ticker.MultipleLocator(20))
        ax.yaxis.set_minor_locator(ticker.MultipleLocator(10))
        ax.grid(axis="y", linestyle="--", linewidth=0.5, alpha=0.6)
        ax.set_ylim(0, 105)

    axes[0].set_ylabel("Tile missing ratio (%)", fontsize=10)

    title = f"TMR por cenário — ABR={abr.upper()}, clientes={clients}, mix={mix}"
    fig.suptitle(title, fontsize=10, y=1.06)

    plt.tight_layout()
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"[OK] salvo em {out_path}")


# ─── Main ─────────────────────────────────────────────────────────────────────

def main() -> None:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("matrix_dir", help="Diretório raiz da matriz (ex: ../../logs/matrix-001)")
    p.add_argument("--abr",     default="bola",     help="ABR a filtrar: bola|legacy (padrão: bola)")
    p.add_argument("--clients", default="6",        help="Número de clientes: 1|6 (padrão: 6)")
    p.add_argument("--mix",     default="balanced", help="Mix FoV: balanced|wide_heavy (padrão: balanced)")
    p.add_argument("-o",        default="",         help="Arquivo de saída (padrão: <matrix_dir>/tmr_by_scenario.png)")
    args = p.parse_args()

    matrix_dir = args.matrix_dir
    if not os.path.isdir(matrix_dir):
        print(f"[erro] diretório não encontrado: {matrix_dir}", file=sys.stderr)
        sys.exit(1)

    out_path = args.o or os.path.join(matrix_dir, "tmr_by_scenario.png")

    print(f"Lendo dados de: {matrix_dir}")
    print(f"Filtro: abr={args.abr} clients={args.clients} mix={args.mix}")

    data = collect(matrix_dir, args.abr, args.clients, args.mix)

    # Resumo do que foi encontrado
    total_points = sum(
        len(v["fov"]) + len(v["nonfov"])
        for sc in data.values()
        for v in sc.values()
    )
    if total_points == 0:
        print("[aviso] nenhum dado encontrado para esse filtro.", file=sys.stderr)
        print("  Verifique se experiment.env tem os campos: scenario_id, policy, abr_mode, num_clients, fov_mix")
        sys.exit(1)

    print(f"Pontos encontrados: {total_points}")
    plot(data, args.abr, args.clients, args.mix, out_path)


if __name__ == "__main__":
    main()
