#!/usr/bin/env python3
"""
Aggregate and plot BOLA vs Legacy in two extreme network conditions.

Output layout is created by run_bola_vs_legacy_extremes.sh:
  <LOG_ROOT>/<condition>/<abr>/experiment.env
  <LOG_ROOT>/<condition>/<abr>/statistics-*.csv

Usage:
  python analyze_bola_legacy_extremes.py <LOG_ROOT>
  python analyze_bola_legacy_extremes.py <LOG_ROOT> --spatial-mix-only

Outputs:
  - <LOG_ROOT>/abr_extremes_summary.csv (fov_mode; zone_*; total_req_nonfov_rows; low_req_* e low_req_nonfov)
  - <LOG_ROOT>/abr_extremes_quality.png
  - <LOG_ROOT>/abr_extremes_dashboard.png (3 painéis)
  - <LOG_ROOT>/abr_extremes_spatial_mix.png (ok=true: 3 zonas; por cenário, barras agrupadas LOW/MED/HIGH)
  - <LOG_ROOT>/abr_extremes_delivered_counts.png

Com --spatial-mix-only, só o spatial_mix.png é escrito (CSV e demais PNGs não).
"""
from __future__ import annotations

import csv
import os
import sys
from collections import defaultdict

ABR_ORDER = ("bola", "legacy")
CONDITION_ORDER = ("good", "bad")

# Zonas espaciais alinhadas ao scheduler: FoV (in_fov), anel perto do FoV (prioridade média), restante.
SPATIAL_ORDER = ("fov", "near_fov", "outside_fov")
SPATIAL_LABELS = {
    "fov": "Dentro do FoV",
    "near_fov": "Perto do FoV",
    "outside_fov": "Fora do FoV",
}
# Rótulos curtos para o PNG dedicado (FoV / perto / background).
ZONE_MIX_LABELS = {
    "fov": "FoV",
    "near_fov": "Perto do FoV",
    "outside_fov": "Background",
}
# Mapeamento igual a server_scheduler_test.sh (--fov narrow|normal|wide)
FOV_TRACE_BY_MODE = {
    "narrow": "user_fov_narrow.csv",
    "normal": "user_fov.csv",
    "wide": "user_fov_wide.csv",
}
BITRATE_ORDER = (3, 5, 10)
BITRATE_LABELS = {3: "LOW", 5: "MED", 10: "HIGH"}


def _parse_bool(v: str) -> bool:
    return str(v).strip().lower() in ("true", "1", "yes")


def _parse_int(row: dict, key: str, default: int) -> int:
    try:
        return int(float(row.get(key, "")))
    except (TypeError, ValueError):
        return default


def spatial_bucket(in_fov: bool, priority: int) -> str:
    """Mapeia (in_fov, priority) para zona espacial (mesma lógica que TileScheduler)."""
    if in_fov:
        return "fov"
    # model: HIGH=0 (in FoV), MEDIUM=1 (near), LOW=2 (far)
    if priority == 1:
        return "near_fov"
    return "outside_fov"


def _parse_env(path: str) -> dict:
    env = {}
    with open(path, encoding="utf-8-sig") as f:
        for line in f:
            line = line.strip()
            if "=" in line and not line.startswith("#"):
                k, v = line.split("=", 1)
                env[k.strip()] = v.strip()
    return env


def _csv_list(dirpath: str) -> list[str]:
    return sorted(
        os.path.join(dirpath, n)
        for n in os.listdir(dirpath)
        if n.startswith("statistics-") and n.endswith(".csv") and "summary" not in n.lower()
    )


def _empty_metrics() -> dict:
    return {
        "rows": 0,
        "ok": 0,
        "on_time": 0,
        "tp_sum": 0.0,
        "fov_mode": "",
        "bitrate_counts": defaultdict(int),
        "ok_bitrate_counts": defaultdict(int),
        # spatial -> bitrate -> count
        "bitrate_counts_spatial": defaultdict(lambda: defaultdict(int)),
        "ok_bitrate_counts_spatial": defaultdict(lambda: defaultdict(int)),
    }


def _accumulate_csv(metrics: dict, csv_paths: list[str]) -> None:
    for path in csv_paths:
        try:
            with open(path, newline="", encoding="utf-8") as f:
                for row in csv.DictReader(f):
                    if _parse_bool(row.get("skipped", "false")):
                        continue
                    metrics["rows"] += 1
                    in_fov = _parse_bool(row.get("in_fov", "false"))
                    priority = _parse_int(row, "priority", 2)
                    zone = spatial_bucket(in_fov, priority)
                    br = None
                    try:
                        br = int(float(row.get("bitrate", "nan")))
                        metrics["bitrate_counts"][br] += 1
                        metrics["bitrate_counts_spatial"][zone][br] += 1
                    except (TypeError, ValueError):
                        br = None
                    if _parse_bool(row.get("ok", "false")):
                        metrics["ok"] += 1
                        if br is not None:
                            metrics["ok_bitrate_counts"][br] += 1
                            metrics["ok_bitrate_counts_spatial"][zone][br] += 1
                    if _parse_bool(row.get("on_time", "false")):
                        metrics["on_time"] += 1
                    try:
                        metrics["tp_sum"] += float(row.get("tp", "0") or 0.0)
                    except ValueError:
                        pass
        except OSError as exc:
            print(f"[warn] {path}: {exc}")


def collect(root: str) -> dict:
    """
    Returns:
      data[condition][abr] = metrics dict
    """
    data = defaultdict(dict)
    for dirpath, _, files in os.walk(root):
        if "experiment.env" not in files:
            continue
        env = _parse_env(os.path.join(dirpath, "experiment.env"))
        if env.get("scenario_family") != "bola_legacy_extremes":
            continue
        condition = env.get("condition_id", "").strip().lower()
        abr = env.get("abr_mode", "").strip().lower()
        if condition not in ("good", "bad"):
            continue
        if abr not in ABR_ORDER:
            continue
        csvs = _csv_list(dirpath)
        if not csvs:
            print(f"[warn] no statistics CSV in {dirpath}")
            continue
        m = _empty_metrics()
        m["fov_mode"] = env.get("fov_mode", "").strip().lower()
        _accumulate_csv(m, csvs)
        data[condition][abr] = m
    return data


def _pct(num: float, den: float) -> float:
    return (100.0 * num / den) if den else 0.0


def _condition_title(condition: str) -> str:
    return "Condição boa" if condition == "good" else "Condição ruim"


def _spatial_summary_fields(m: dict) -> dict:
    """Colunas CSV/plot: contagens e % LOW/MED/HIGH dentro de cada zona espacial."""
    out: dict = {}
    for zone in SPATIAL_ORDER:
        pref = f"zone_{zone}_"
        counts = m["bitrate_counts_spatial"][zone]
        tot = sum(counts.values())
        out[pref + "rows"] = int(tot)
        for br in BITRATE_ORDER:
            out[pref + f"br{br}_pct"] = _pct(counts.get(br, 0), tot)
    out["total_req_nonfov_rows"] = int(out["zone_near_fov_rows"]) + int(out["zone_outside_fov_rows"])
    return out


def _delivered_spatial_counts(m: dict) -> dict:
    """Contagens com ok=true por zona (qualquer bitrate registrado no CSV)."""
    fov = sum(m["ok_bitrate_counts_spatial"]["fov"].values())
    near = sum(m["ok_bitrate_counts_spatial"]["near_fov"].values())
    out = sum(m["ok_bitrate_counts_spatial"]["outside_fov"].values())
    return {
        "delivered_ok_fov": int(fov),
        "delivered_ok_near_fov": int(near),
        "delivered_ok_outside_fov": int(out),
    }


def _delivered_ok_zone_bitrate_fields(m: dict) -> dict:
    """Contagens ok=true por (zona espacial, bitrate): colunas delivered_ok_<zone>_<low|med|high>."""
    tier = {3: "low", 5: "med", 10: "high"}
    out: dict = {}
    for zone in SPATIAL_ORDER:
        for br in BITRATE_ORDER:
            name = tier[br]
            out[f"delivered_ok_{zone}_{name}"] = int(m["ok_bitrate_counts_spatial"][zone].get(br, 0))
    return out
    """Contagens com ok=true por zona (qualquer bitrate registrado no CSV)."""
    fov = sum(m["ok_bitrate_counts_spatial"]["fov"].values())
    near = sum(m["ok_bitrate_counts_spatial"]["near_fov"].values())
    out = sum(m["ok_bitrate_counts_spatial"]["outside_fov"].values())
    return {
        "delivered_ok_fov": int(fov),
        "delivered_ok_near_fov": int(near),
        "delivered_ok_outside_fov": int(out),
    }


def _low_request_counts(m: dict) -> dict:
    """Contagens exatas de requisições com bitrate LOW por zona espacial."""
    out = {
        "low_req_fov": int(m["bitrate_counts_spatial"]["fov"].get(3, 0)),
        "low_req_near_fov": int(m["bitrate_counts_spatial"]["near_fov"].get(3, 0)),
        "low_req_outside_fov": int(m["bitrate_counts_spatial"]["outside_fov"].get(3, 0)),
    }
    out["low_req_total"] = out["low_req_fov"] + out["low_req_near_fov"] + out["low_req_outside_fov"]
    out["low_req_nonfov"] = out["low_req_near_fov"] + out["low_req_outside_fov"]
    return out


def _mix_zone_of_total_fields(m: dict) -> dict:
    """
    Para cada bitrate, % de *todas* as requisições naquela zona+bitrate.
    Soma_z mix_{low|med|high}_{zone}_pct = bitrate_{low|med|high}_pct global.
    """
    total = m["rows"]
    tier = {3: "low", 5: "med", 10: "high"}
    out: dict = {}
    for br, name in tier.items():
        for zone in SPATIAL_ORDER:
            c = m["bitrate_counts_spatial"][zone].get(br, 0)
            out[f"mix_{name}_{zone}_of_total_pct"] = _pct(c, total)
    return out


def write_summary_csv(data: dict, out_csv: str, *, write_disk: bool = True) -> list[dict]:
    rows = []
    for condition in CONDITION_ORDER:
        for abr in ABR_ORDER:
            m = data.get(condition, {}).get(abr)
            if not m:
                continue
            total = m["rows"]
            bit = m["bitrate_counts"]
            item = {
                "condition": condition,
                "abr": abr,
                "fov_mode": m.get("fov_mode", ""),
                "rows": total,
                "ok_rows": m["ok"],
                "ok_rate_pct": _pct(m["ok"], total),
                "tile_missing_pct": _pct(total - m["ok"], total),
                "on_time_pct": _pct(m["on_time"], total),
                "avg_tp": (m["tp_sum"] / total) if total else 0.0,
                "bitrate_low_pct": _pct(bit.get(3, 0), total),
                "bitrate_med_pct": _pct(bit.get(5, 0), total),
                "bitrate_high_pct": _pct(bit.get(10, 0), total),
                "bitrate_non_low_pct": _pct(bit.get(5, 0) + bit.get(10, 0), total),
                "delivered_low_count": int(m["ok_bitrate_counts"].get(3, 0)),
                "delivered_med_count": int(m["ok_bitrate_counts"].get(5, 0)),
                "delivered_high_count": int(m["ok_bitrate_counts"].get(10, 0)),
            }
            item.update(_delivered_spatial_counts(m))
            item.update(_delivered_ok_zone_bitrate_fields(m))
            item.update(_spatial_summary_fields(m))
            item.update(_mix_zone_of_total_fields(m))
            item.update(_low_request_counts(m))
            rows.append(item)

    os.makedirs(os.path.dirname(out_csv) or ".", exist_ok=True)
    base_fields = [
        "condition",
        "abr",
        "fov_mode",
        "rows",
        "ok_rows",
        "ok_rate_pct",
        "tile_missing_pct",
        "on_time_pct",
        "avg_tp",
        "bitrate_low_pct",
        "bitrate_med_pct",
        "bitrate_high_pct",
        "bitrate_non_low_pct",
        "delivered_low_count",
        "delivered_med_count",
        "delivered_high_count",
        "delivered_ok_fov",
        "delivered_ok_near_fov",
        "delivered_ok_outside_fov",
    ]
    tier_names = {3: "low", 5: "med", 10: "high"}
    zone_bitrate_fields: list[str] = []
    for zone in SPATIAL_ORDER:
        for br in BITRATE_ORDER:
            zone_bitrate_fields.append(f"delivered_ok_{zone}_{tier_names[br]}")
    base_fields.extend(zone_bitrate_fields)
    base_fields.extend(
        [
            "low_req_total",
            "low_req_fov",
            "low_req_near_fov",
            "low_req_outside_fov",
            "low_req_nonfov",
        ]
    )
    spatial_fields: list[str] = []
    for zone in SPATIAL_ORDER:
        pref = f"zone_{zone}_"
        spatial_fields.append(pref + "rows")
        for br in BITRATE_ORDER:
            spatial_fields.append(pref + f"br{br}_pct")
    spatial_fields.append("total_req_nonfov_rows")
    tier = {3: "low", 5: "med", 10: "high"}
    mix_fields: list[str] = []
    for br in BITRATE_ORDER:
        name = tier[br]
        for zone in SPATIAL_ORDER:
            mix_fields.append(f"mix_{name}_{zone}_of_total_pct")
    fieldnames = base_fields + spatial_fields + mix_fields
    if write_disk:
        os.makedirs(os.path.dirname(out_csv) or ".", exist_ok=True)
        with open(out_csv, "w", newline="", encoding="utf-8") as f:
            w = csv.DictWriter(f, fieldnames=fieldnames)
            w.writeheader()
            for r in rows:
                w.writerow(r)
        print(f"Saved: {out_csv}")
    return rows


def _save_fig(fig, out_path: str) -> None:
    import matplotlib.pyplot as plt

    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    fig.savefig(out_path, dpi=160, bbox_inches="tight", pad_inches=0.5)
    plt.close(fig)
    print(f"Saved: {out_path}")


def _ylim_for_bar_labels(ymax: float, *, cap: float = 110.0) -> float:
    """Espaço extra no topo para rótulos acima das barras (valores em %)."""
    if ymax <= 0:
        return 14.0
    head = max(6.0, ymax * 0.10)
    return min(cap, ymax + head)


def plot_quality(rows: list[dict], out_path: str) -> None:
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        print("[warn] no rows for quality plot")
        return

    row_map = {(r["condition"], r["abr"]): r for r in rows}
    x = np.arange(len(CONDITION_ORDER), dtype=float)
    width = 0.36
    abr_colors = {"bola": "#2563eb", "legacy": "#c2410c"}

    fig, axes = plt.subplots(1, 2, figsize=(13.2, 5.2))
    for ax, metric, ylab, title in (
        (axes[0], "tile_missing_pct", "Tile missing ratio (%)", "Perda de tiles"),
        (axes[1], "on_time_pct", "On-time ratio (%)", "Entrega dentro do deadline"),
    ):
        all_vals = []
        for i, abr in enumerate(ABR_ORDER):
            vals = []
            for cond in CONDITION_ORDER:
                r = row_map.get((cond, abr))
                vals.append(float(r[metric]) if r else 0.0)
            all_vals.extend(vals)
            bars = ax.bar(
                x + (i - 0.5) * width,
                vals,
                width,
                label=("BOLA" if abr == "bola" else "Legacy"),
                color=abr_colors[abr],
                edgecolor="#111827",
                linewidth=0.5,
            )
            ax.bar_label(
                bars,
                labels=[f"{v:.2f}%" for v in vals],
                padding=3,
                fontsize=8.5,
                color="#1e293b",
            )
            # No painel LOW(%), também explicita MED/HIGH na própria barra.
            if metric == "bitrate_low_pct":
                for bar_idx, cond in enumerate(CONDITION_ORDER):
                    rr = row_map.get((cond, abr))
                    if not rr:
                        continue
                    med = float(rr.get("bitrate_med_pct", 0.0))
                    high = float(rr.get("bitrate_high_pct", 0.0))
                    v = vals[bar_idx]
                    ax.text(
                        bars[bar_idx].get_x() + bars[bar_idx].get_width() / 2.0,
                        max(3.0, v * 0.55),
                        f"M {med:.2f}%\nH {high:.2f}%",
                        ha="center",
                        va="center",
                        fontsize=7.8,
                        color="#f8fafc",
                        linespacing=1.15,
                        fontweight="bold",
                    )
        ax.set_xticks(x)
        ax.set_xticklabels([_condition_title(c) for c in CONDITION_ORDER], fontsize=11)
        ax.set_ylabel(ylab, fontsize=11)
        ymax = max(all_vals) if all_vals else 0.0
        ax.set_ylim(0, _ylim_for_bar_labels(ymax))
        ax.set_title(title, fontweight="bold", fontsize=12)
        ax.grid(axis="y", linestyle="--", alpha=0.4)
        ax.set_axisbelow(True)
        ax.tick_params(axis="y", labelsize=10)

    handles, labels = axes[0].get_legend_handles_labels()
    fig.suptitle("BOLA vs Legacy — Qualidade (Mininet)", fontsize=13, fontweight="bold", y=0.98)
    fig.legend(handles, labels, loc="upper center", bbox_to_anchor=(0.5, 0.02), ncol=2, frameon=True, fancybox=False, edgecolor="#cbd5e1")
    fig.subplots_adjust(left=0.07, right=0.98, top=0.80, bottom=0.22, wspace=0.26)
    _save_fig(fig, out_path)


def plot_dashboard(rows: list[dict], out_path: str) -> None:
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        print("[warn] no rows for dashboard plot")
        return
    row_map = {(r["condition"], r["abr"]): r for r in rows}
    x = np.arange(len(CONDITION_ORDER), dtype=float)
    width = 0.36
    colors = {"bola": "#2563eb", "legacy": "#c2410c"}
    metrics = [
        ("tile_missing_pct", "Tile missing (%)\n(menor é melhor)", True),
        ("on_time_pct", "On-time (%)\n(maior é melhor)", False),
        ("bitrate_low_pct", "LOW no total (%)\n(conservadorismo do ABR)", False),
    ]

    fig, flat_axes = plt.subplots(1, 3, figsize=(13.2, 5.0))
    for ax, (metric, title, _lower_note) in zip(flat_axes, metrics):
        all_vals = []
        for i, abr in enumerate(ABR_ORDER):
            vals = []
            for cond in CONDITION_ORDER:
                r = row_map.get((cond, abr))
                vals.append(float(r[metric]) if r else 0.0)
            all_vals.extend(vals)
            bars = ax.bar(
                x + (i - 0.5) * width,
                vals,
                width,
                label=("BOLA" if abr == "bola" else "Legacy"),
                color=colors[abr],
                edgecolor="#111827",
                linewidth=0.5,
            )
            ax.bar_label(
                bars,
                labels=[f"{v:.2f}%" for v in vals],
                padding=3,
                fontsize=9,
                color="#1e293b",
            )

        ax.set_xticks(x)
        ax.set_xticklabels([_condition_title(c) for c in CONDITION_ORDER], fontsize=11)
        ax.set_title(title, fontweight="bold", fontsize=11)
        ax.grid(axis="y", linestyle="--", alpha=0.4)
        ax.set_axisbelow(True)
        ymax = max(all_vals) if all_vals else 0.0
        ax.set_ylim(0, _ylim_for_bar_labels(ymax))
        ax.tick_params(axis="y", labelsize=10)

    handles, labels = flat_axes[0].get_legend_handles_labels()
    fig.suptitle(
        "BOLA vs Legacy — Painel comparativo (condições extremas de rede)",
        fontsize=13,
        fontweight="bold",
        y=1.02,
    )
    fig.legend(
        handles,
        labels,
        loc="upper center",
        bbox_to_anchor=(0.5, 0.02),
        ncol=2,
        frameon=True,
        fancybox=False,
        edgecolor="#cbd5e1",
    )
    fig.subplots_adjust(left=0.07, right=0.99, top=0.86, bottom=0.20, wspace=0.28)
    _save_fig(fig, out_path)


def plot_spatial_mix(rows: list[dict], out_path: str) -> None:
    """
    Três faixas (FoV / perto / fundo). Em cada uma: por cenário de rede×ABR,
    três barras lado a lado (LOW, MED, HIGH) — contagens ok=true. Eixo Y próprio
    por faixa. Legenda à direita da figura; números só por cima das barras (>0).
    """
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        print("[warn] no rows for spatial mix plot")
        return

    row_map = {(r["condition"], r["abr"]): r for r in rows}
    keys: list[tuple[str, str]] = []
    for cond in CONDITION_ORDER:
        for abr in ABR_ORDER:
            keys.append((cond, abr))

    tier_key = {3: "low", 5: "med", 10: "high"}
    tier_plot = (
        (3, "LOW", "#2563eb"),
        (5, "MED", "#ca8a04"),
        (10, "HIGH", "#15803d"),
    )
    bar_short = ("Boa+BOLA", "Boa+Legacy", "Ruim+BOLA", "Ruim+Legacy")
    zone_title = {
        "fov": "Dentro do FoV",
        "near_fov": "Perto do FoV",
        "outside_fov": "Fundo (fora)",
    }

    n_bar = len(keys)
    x = np.arange(n_bar, dtype=float)
    w = 0.21

    fig, axes = plt.subplots(len(SPATIAL_ORDER), 1, figsize=(13.5, 10.2), sharex=True)
    if len(SPATIAL_ORDER) == 1:
        axes = [axes]

    for row_ax, zone in zip(axes, SPATIAL_ORDER):
        ymax = 1.0
        for zi, (br, lbl, color) in enumerate(tier_plot):
            offs = (zi - 1.0) * w
            heights = []
            for cond, abr in keys:
                r = row_map.get((cond, abr))
                k = f"delivered_ok_{zone}_{tier_key[br]}"
                heights.append(float(int(r.get(k, 0) or 0)) if r else 0.0)
            hs = np.array(heights, dtype=float)
            bars = row_ax.bar(
                x + offs,
                hs,
                w * 0.92,
                label=lbl,
                color=color,
                edgecolor="#111827",
                linewidth=0.55,
            )
            lbls = [str(int(h)) if h > 0 else "" for h in hs]
            row_ax.bar_label(bars, labels=lbls, fontsize=8, padding=2, color="#1e293b", fontweight="bold")
            mh = float(np.max(hs)) if len(hs) else 0.0
            if mh > ymax:
                ymax = mh

        row_ax.set_ylim(0, max(ymax * 1.18, 6.0))
        row_ax.set_ylabel("Entregas ok=true", fontsize=10)
        row_ax.set_title(zone_title[zone], fontsize=11.5, fontweight="bold", loc="left")
        row_ax.grid(axis="y", linestyle="--", alpha=0.3)
        row_ax.set_axisbelow(True)

    bottom_ax = axes[-1]
    bottom_ax.set_xticks(x)
    bottom_ax.set_xticklabels(bar_short, fontsize=10)
    bottom_ax.tick_params(axis="x", pad=6)

    handles, labels = axes[0].get_legend_handles_labels()
    fig.suptitle(
        "Entregas ok=true por zona e bitrate (barras agrupadas)",
        fontsize=13,
        fontweight="bold",
        y=0.98,
    )
    fig.text(0.5, 0.935, _fov_trace_caption(rows), ha="center", fontsize=9, color="#475569")
    fig.legend(
        handles,
        labels,
        loc="center left",
        bbox_to_anchor=(0.91, 0.52),
        fontsize=10,
        frameon=True,
        fancybox=False,
        edgecolor="#cbd5e1",
    )
    fig.text(
        0.5,
        0.02,
        "Colunas de bitrate por cenário; valores detalhados: CSV (delivered_ok_<zona>_low|med|high).",
        ha="center",
        fontsize=8.5,
        color="#64748b",
    )
    fig.subplots_adjust(left=0.07, right=0.88, top=0.90, bottom=0.10, hspace=0.34)
    _save_fig(fig, out_path)


def _fov_trace_caption(rows: list[dict]) -> str:
    modes = {str(r.get("fov_mode", "")).strip().lower() for r in rows if r}
    modes.discard("")
    if len(modes) == 1:
        mode = next(iter(modes))
        fname = FOV_TRACE_BY_MODE.get(mode, "?")
        return f"Matriz de FoV: {fname}   (fov_mode={mode} — mesmo mapeamento que server_scheduler_test.sh)"
    if not modes:
        return "Matriz de FoV: (fov_mode ausente no experiment.env)"
    return f"Matriz de FoV: modos mistos {sorted(modes)} — ver coluna fov_mode no CSV"


def plot_delivered_counts(rows: list[dict], out_path: str) -> None:
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        print("[warn] no rows for delivered-count plot")
        return

    row_map = {(r["condition"], r["abr"]): r for r in rows}
    labels = []
    low = []
    med = []
    high = []
    for cond in CONDITION_ORDER:
        for abr in ABR_ORDER:
            rr = row_map.get((cond, abr))
            if not rr:
                continue
            labels.append(f"{_condition_title(cond)}\n{('BOLA' if abr == 'bola' else 'Legacy')}")
            low.append(int(rr["delivered_low_count"]))
            med.append(int(rr["delivered_med_count"]))
            high.append(int(rr["delivered_high_count"]))

    x = np.arange(len(labels), dtype=float)
    width = 0.25
    fig, ax = plt.subplots(figsize=(11.8, 5.4))
    b1 = ax.bar(x - width, low, width, label="LOW entregue", color="#2563eb", edgecolor="#111827", linewidth=0.5)
    b2 = ax.bar(x, med, width, label="MED entregue", color="#ca8a04", edgecolor="#111827", linewidth=0.5)
    b3 = ax.bar(x + width, high, width, label="HIGH entregue", color="#15803d", edgecolor="#111827", linewidth=0.5)

    ax.set_xticks(x)
    ax.set_xticklabels(labels, fontsize=10.5)
    ax.set_ylabel("Quantidade entregue (ok=true)")
    ax.set_title("Contagem entregue por bitrate (LOW/MED/HIGH)", fontweight="bold")
    ax.grid(axis="y", linestyle="--", alpha=0.4)
    ax.set_axisbelow(True)
    # Escala log para permitir visualizar MED/HIGH quando LOW domina.
    ax.set_yscale("log")
    ax.set_ylim(0.8, max(2.0, max(low + med + high) * 1.8))
    ax.legend(loc="upper right", fontsize=9.5)

    # Labels numéricos nas barras
    for bars in (b1, b2, b3):
        ax.bar_label(bars, labels=[str(int(b.get_height())) for b in bars], padding=2, fontsize=8, color="#1e293b")

    fig.text(0.5, 0.015, "Escala Y em log para evidenciar MED/HIGH com LOW dominante.", ha="center", fontsize=8.5, color="#475569")
    fig.subplots_adjust(bottom=0.20, top=0.86)
    _save_fig(fig, out_path)


def main() -> None:
    import argparse

    parser = argparse.ArgumentParser(description="Agrega logs bola_legacy_extremes e gera CSV/PNG.")
    parser.add_argument("log_root", help="Pasta raiz com subpastas condition/abr/")
    parser.add_argument(
        "--spatial-mix-only",
        action="store_true",
        help="Gera só abr_extremes_spatial_mix.png (não grava CSV nem os outros PNGs).",
    )
    args = parser.parse_args()
    root = args.log_root
    if not os.path.isdir(root):
        print(f"Not a directory: {root}")
        sys.exit(1)

    data = collect(root)
    if not data:
        print("No runs found for scenario_family=bola_legacy_extremes")
        sys.exit(2)

    out_csv = os.path.join(root, "abr_extremes_summary.csv")
    rows = write_summary_csv(data, out_csv, write_disk=not args.spatial_mix_only)
    if not rows:
        print("No valid rows to summarize.")
        sys.exit(3)

    mix_png = os.path.join(root, "abr_extremes_spatial_mix.png")
    if args.spatial_mix_only:
        plot_spatial_mix(rows, mix_png)
        return

    plot_quality(rows, os.path.join(root, "abr_extremes_quality.png"))
    plot_dashboard(rows, os.path.join(root, "abr_extremes_dashboard.png"))
    plot_spatial_mix(rows, mix_png)
    plot_delivered_counts(rows, os.path.join(root, "abr_extremes_delivered_counts.png"))


if __name__ == "__main__":
    main()
