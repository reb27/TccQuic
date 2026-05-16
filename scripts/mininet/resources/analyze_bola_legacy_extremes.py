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
  - <LOG_ROOT>/abr_extremes_spatial_mix.png (contagens ok=true por zona FoV/perto/fundo; barras agrupadas + escala log)
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
        "low_req_total",
        "low_req_fov",
        "low_req_near_fov",
        "low_req_outside_fov",
        "low_req_nonfov",
    ]
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
    Barras agrupadas por cenário: contagem de entregas ok=true em cada zona
    (FoV / perto / fundo), qualquer bitrate. Eixo Y em log para não esmagar
    FoV/perto quando o fundo domina (cenário típico do dataset).
    """
    import matplotlib.pyplot as plt
    import numpy as np

    if not rows:
        print("[warn] no rows for spatial mix plot")
        return

    row_map = {(r["condition"], r["abr"]): r for r in rows}
    bar_short = ("Boa / BOLA", "Boa / Legacy", "Ruim / BOLA", "Ruim / Legacy")
    abb = ("BB", "BL", "RB", "RL")

    zone_style = {
        "fov": ("FoV", "#14b8a6"),
        "near_fov": ("Perto do FoV", "#f59e0b"),
        "outside_fov": ("Fundo", "#64748b"),
    }

    keys: list[tuple[str, str]] = []
    for cond in CONDITION_ORDER:
        for abr in ABR_ORDER:
            keys.append((cond, abr))

    def _zone_counts(r: dict | None) -> tuple[int, int, int, int]:
        if not r:
            return (0, 0, 0, 0)
        cv = int(r.get("delivered_ok_fov", 0) or 0)
        cn = int(r.get("delivered_ok_near_fov", 0) or 0)
        co = int(r.get("delivered_ok_outside_fov", 0) or 0)
        ct = cv + cn + co
        return (cv, cn, co, ct)

    fig = plt.figure(figsize=(12.2, 7.2))
    gs = fig.add_gridspec(2, 1, height_ratios=[3.45, 1.12], hspace=0.30)
    ax = fig.add_subplot(gs[0, 0])
    ax_tbl = fig.add_subplot(gs[1, 0])
    ax_tbl.axis("off")

    n = len(keys)
    x = np.arange(n, dtype=float)
    n_z = len(SPATIAL_ORDER)
    group_w = 0.72
    bar_w = group_w / n_z
    offsets = (np.arange(n_z, dtype=float) - (n_z - 1) / 2.0) * bar_w

    max_pos = 1.0
    for cond, abr in keys:
        cv, cn, co, _ = _zone_counts(row_map.get((cond, abr)))
        for v in (cv, cn, co):
            if v > max_pos:
                max_pos = float(v)

    for zi, zone in enumerate(SPATIAL_ORDER):
        lbl, color = zone_style[zone]
        counts = []
        for cond, abr in keys:
            r = row_map.get((cond, abr))
            cv, cn, co, _ = _zone_counts(r)
            czone = {"fov": cv, "near_fov": cn, "outside_fov": co}[zone]
            counts.append(czone)
        c_arr = np.array(counts, dtype=float)
        plot_h = np.where(c_arr > 0, c_arr, np.nan)
        ax.bar(
            x + offsets[zi],
            plot_h,
            bar_w * 0.92,
            label=lbl,
            color=color,
            edgecolor="#0f172a",
            linewidth=0.65,
        )
        for j, raw in enumerate(c_arr):
            ri = int(raw)
            if ri <= 0:
                continue
            y_txt = float(raw) * 1.12
            ax.text(
                float(x[j] + offsets[zi]),
                y_txt,
                f"{ri:,}".replace(",", "."),
                ha="center",
                va="bottom",
                fontsize=8,
                fontweight="bold",
                color="#0f172a",
            )

    mismatch_note = False
    xt_lbl = []
    for j, (cond, abr) in enumerate(keys):
        r = row_map.get((cond, abr))
        ok_rows = int(r["ok_rows"]) if r else 0
        _, _, _, ct = _zone_counts(r)
        if r and ct != ok_rows:
            mismatch_note = True
        xt_lbl.append(f"{bar_short[j]}\n(ok=true total {ok_rows})")

    ax.set_xticks(x)
    ax.set_xticklabels(xt_lbl, fontsize=10)
    ax.set_ylabel("Quantidade entregue (ok=true), escala log₁₀", fontsize=11)
    ax.set_yscale("log")
    ax.set_ylim(0.35, max(max_pos * 2.2, 10.0))
    y_lo, _y_hi = ax.get_ylim()

    for zi, _ in enumerate(SPATIAL_ORDER):
        counts = []
        for cond, abr in keys:
            r = row_map.get((cond, abr))
            cv, cn, co, _ = _zone_counts(r)
            czone = {"fov": cv, "near_fov": cn, "outside_fov": co}[zone]
            counts.append(czone)
        for j, raw in enumerate(counts):
            if raw > 0:
                continue
            ax.text(
                float(x[j] + offsets[zi]),
                y_lo * 1.35,
                "0",
                ha="center",
                va="bottom",
                fontsize=7.5,
                color="#64748b",
            )

    ax.set_title(
        "Entregas bem-sucedidas por zona espacial (contagem absoluta)\n"
        + _fov_trace_caption(rows),
        fontsize=12,
        fontweight="bold",
        pad=10,
    )
    ax.grid(axis="y", linestyle="--", alpha=0.28)
    ax.set_axisbelow(True)
    ax.legend(
        loc="lower center",
        bbox_to_anchor=(0.5, 1.02),
        ncol=3,
        fontsize=9.5,
        framealpha=0.95,
        columnspacing=1.0,
    )

    for j in range(n):
        r = row_map.get(keys[j])
        _, _, _, ct = _zone_counts(r)
        if r and ct <= 0:
            ax.text(
                float(x[j]),
                y_lo * 1.6,
                "sem entregas\nok neste run",
                ha="center",
                va="bottom",
                fontsize=8.5,
                color="#94a3b8",
            )

    tbl_headers = ["", "FoV", "Perto", "Fundo", "Σ zonas", "ok_rows"]
    tbl_rows: list[list[str]] = []
    for j, (cond, abr) in enumerate(keys):
        r = row_map.get((cond, abr))
        if not r:
            continue
        cv, cn, co, ct = _zone_counts(r)
        ok_rows = int(r["ok_rows"])
        tbl_rows.append([abb[j], str(cv), str(cn), str(co), str(ct), str(ok_rows)])

    table = ax_tbl.table(
        cellText=tbl_rows,
        colLabels=tbl_headers,
        loc="center",
        cellLoc="center",
    )
    table.auto_set_font_size(False)
    table.set_fontsize(9)
    table.scale(1.06, 1.9)
    for k in range(len(tbl_headers)):
        table[(0, k)].set_facecolor("#e2e8f0")
        table[(0, k)].set_text_props(fontweight="bold")

    foot = (
        "Contagens = tiles ok=true com bitrate válido, por zona. Σ zonas deve coincidir com ok_rows; "
        "se diferir, há ok=true sem bitrate no CSV. Eixo log evita barra única dominante."
    )
    if mismatch_note:
        foot += " Atenção: Σ zonas ≠ ok_rows em pelo menos um cenário."
    fig.text(0.5, 0.02, foot, ha="center", fontsize=8.3, color="#64748b")

    fig.subplots_adjust(left=0.09, right=0.97, top=0.78, bottom=0.13)
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
