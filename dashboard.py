import argparse
import csv
import glob
import json
import os
import re
from collections import defaultdict


SERVER_MODES = {"fifo", "sp", "wfq"}
ABR_ALIASES = {
    "bola": "bola",
    "bola_finite": "bola",
    "bolafinite": "bola",
    "legacy": "legacy",
    "default": "legacy",
    "threshold": "legacy",
    "abr": "legacy",
}
FOV_MODES = {"narrow", "normal", "wide"}
SUMMARY_METRICS = [
    {
        "key": "deadline_miss_rate_fov_percent",
        "label": "FoV deadline miss",
        "unit": "%",
        "better": "lower",
    },
    {
        "key": "deadline_miss_rate_nonfov_percent",
        "label": "Non-FoV deadline miss",
        "unit": "%",
        "better": "lower",
    },
    {
        "key": "fov_hit_rate_delivery_percent",
        "label": "FoV hit rate",
        "unit": "%",
        "better": "higher",
    },
    {
        "key": "useful_goodput_fov_kbps",
        "label": "FoV useful goodput",
        "unit": "kbps",
        "better": "higher",
    },
    {
        "key": "timely_bytes_ratio_percent",
        "label": "Timely bytes",
        "unit": "%",
        "better": "higher",
    },
    {
        "key": "stale_bytes_ratio_percent",
        "label": "Stale bytes",
        "unit": "%",
        "better": "lower",
    },
]

GENERIC_ABR_RE = re.compile(
    r"ABR: cfg=(?P<cfg>\S+) "
    r"fov_bitrate=(?P<fov_bitrate>\d+) "
    r"nonfov_bitrate=(?P<nonfov_bitrate>\d+) "
    r"avg_tp=(?P<avg_tp>[-\d.]+) "
    r"buffer=(?P<buffer_s>[-\d.]+) s"
)
BOLA_ABR_RE = re.compile(
    r"ABR \(BOLA\): seg=(?P<segment>\d+) "
    r"cfg=(?P<cfg>\S+) "
    r"fov=(?P<fov_bitrate>\d+) "
    r"nonfov=(?P<nonfov_bitrate>\d+) "
    r"buf=(?P<buffer_s>[-\d.]+)s "
    r"thr=(?P<avg_tp_kbps>[-\d.]+) kbps "
    r"budget=(?P<budget_kb>[-\d.]+) KB "
    r"size=(?P<size_kb>[-\d.]+) KB "
    r"score=(?P<score>[-\d.]+) "
    r"guardrail=(?P<guardrail>true|false) "
    r"uMax=(?P<u_max>[-\d.]+) "
    r"V=(?P<v_value>[-\d.]+) "
    r"Q=(?P<q_value>[-\d.]+)"
)


def load_csv(path):
    if not path or not os.path.exists(path):
        return []
    with open(path, newline="", encoding="utf-8") as handle:
        return list(csv.DictReader(handle))


def first_match(pattern):
    matches = sorted(glob.glob(pattern))
    return matches[0] if matches else None


def parse_bool(value):
    return str(value).strip().lower() in {"1", "true", "yes"}


def parse_int(value, default=None):
    if value in (None, ""):
        return default
    try:
        return int(str(value).strip())
    except (TypeError, ValueError):
        return default


def parse_float(value, default=0.0):
    if value in (None, ""):
        return default
    try:
        return float(str(value).strip())
    except (TypeError, ValueError):
        return default


def normalize_abr_mode(value):
    raw = str(value or "").strip().lower()
    return ABR_ALIASES.get(raw, raw or "unknown")


def parse_experiment_env(path):
    data = {}
    if not path or not os.path.exists(path):
        return data
    with open(path, encoding="utf-8") as handle:
        for raw_line in handle:
            line = raw_line.strip()
            if not line or "=" not in line:
                continue
            key, value = line.split("=", 1)
            data[key.strip()] = value.strip()
    return data


def parse_metadata_from_name(name):
    lower = name.lower()
    tokens = re.split(r"[_\-/]+", lower)
    meta = {}

    for token in tokens:
        if token in SERVER_MODES:
            meta["server_mode"] = token
        abr = normalize_abr_mode(token)
        if abr in {"bola", "legacy"}:
            meta["abr_mode"] = abr
        if token in FOV_MODES:
            meta["fov_mode"] = token

    patterns = {
        "scenario": r"scenario(\d+)",
        "base_latency_ms": r"baselatency(\d+)",
        "server_bw_mbps": r"sbw(\d+)",
        "client_bw_mbps": r"cbw(\d+)",
    }
    for key, pattern in patterns.items():
        match = re.search(pattern, lower)
        if match:
            meta[key] = parse_int(match.group(1))

    return meta


def parse_metadata_from_path(relative_path):
    normalized = relative_path.replace("\\", "/")
    parts = [part for part in normalized.split("/") if part]
    meta = {}

    for part in parts:
        lower = part.lower()
        if lower in SERVER_MODES:
            meta["server_mode"] = lower
        abr = normalize_abr_mode(lower)
        if abr in {"bola", "legacy"}:
            meta["abr_mode"] = abr
        if lower in FOV_MODES:
            meta["fov_mode"] = lower
        match = re.match(r"scenario(\d+)-baselatency(\d+)", lower)
        if match:
            meta["scenario"] = parse_int(match.group(1))
            meta["base_latency_ms"] = parse_int(match.group(2))
        if lower.startswith("scenario") and "scenario" not in meta:
            meta["scenario"] = parse_int(lower.replace("scenario", ""))
        if lower.startswith("net") and "scenario" not in meta:
            meta["scenario"] = lower

    return meta


def build_run_metadata(base_dir, run_dir, files):
    rel_path = os.path.relpath(run_dir, base_dir)
    rel_path = "." if rel_path == "." else rel_path.replace("\\", "/")

    meta = {}
    meta.update(parse_metadata_from_name(os.path.basename(run_dir)))
    meta.update(parse_metadata_from_path(rel_path))

    env_data = parse_experiment_env(files.get("experiment_env"))
    if env_data:
        env_map = {
            "scenario": ("scenario", parse_int),
            "server_mode": ("server_mode", str),
            "abr_mode": ("abr_mode", normalize_abr_mode),
            "base_latency_ms": ("base_latency_ms", parse_int),
            "delay_ms": ("delay_ms", parse_int),
            "loss_pct": ("loss_pct", parse_float),
            "background_load_pct": ("background_load_pct", parse_float),
            "server_bw_mbps": ("server_bw_mbps", parse_int),
            "client_bw_mbps": ("client_bw_mbps", parse_int),
            "parallelism": ("parallelism", parse_int),
            "fov_mode": ("fov_mode", str),
        }
        for env_key, (meta_key, caster) in env_map.items():
            if env_key not in env_data:
                continue
            value = caster(env_data[env_key])
            if value not in (None, ""):
                meta[meta_key] = value

    meta["id"] = rel_path
    meta["meta_partial"] = not bool(env_data)
    meta["fov_mode"] = str(meta.get("fov_mode") or "normal").lower()
    meta["server_mode"] = str(meta.get("server_mode") or "unknown").lower()
    meta["abr_mode"] = normalize_abr_mode(meta.get("abr_mode"))

    return meta


def find_run_files(run_dir):
    files = {
        "experiment_env": first_match(os.path.join(run_dir, "experiment.env")),
        "statistics_summary": first_match(os.path.join(run_dir, "statistics-summary*.csv")),
        "statistics": None,
        "fov_delivery": first_match(os.path.join(run_dir, "fov-delivery-*.csv")),
        "fov_goodput": first_match(os.path.join(run_dir, "fov-goodput-*.csv")),
        "deadline_lateness": first_match(os.path.join(run_dir, "deadline-lateness-*.csv")),
        "wfq_utilization": first_match(os.path.join(run_dir, "wfq_utilization.csv")),
        "stdout": first_match(os.path.join(run_dir, "stdout")),
    }

    stats_candidates = sorted(glob.glob(os.path.join(run_dir, "statistics-*.csv")))
    for candidate in stats_candidates:
        if "summary" not in os.path.basename(candidate):
            files["statistics"] = candidate
            break

    return files


def is_run_directory(run_dir):
    files = find_run_files(run_dir)
    has_client_data = any(
        files.get(key)
        for key in (
            "statistics_summary",
            "statistics",
            "fov_delivery",
            "fov_goodput",
            "deadline_lateness",
            "stdout",
        )
    )
    return has_client_data, files


def normalize_summary(path):
    rows = load_csv(path)
    if not rows:
        return {}
    row = rows[0]
    normalized = {}
    for key, value in row.items():
        normalized[key] = parse_float(value, default=value)
    return normalized


def first_segment_hint(fov_delivery_rows, statistics_rows):
    if fov_delivery_rows:
        return parse_int(fov_delivery_rows[0].get("segment"), 1)
    if statistics_rows:
        segments = [parse_int(row.get("segment")) for row in statistics_rows]
        segments = [value for value in segments if value is not None]
        if segments:
            return min(segments)
    return 1


def normalize_fov_delivery(path):
    rows = load_csv(path)
    series = []
    for row in rows:
        series.append(
            {
                "segment": parse_int(row.get("segment"), 0),
                "fov_tiles": parse_int(row.get("fov_tiles"), 0),
                "fov_on_time": parse_int(row.get("fov_on_time"), 0),
                "fov_hit_rate_percent": parse_float(row.get("fov_hit_rate_percent"), 0.0),
            }
        )
    return series


def normalize_fov_goodput(path):
    rows = load_csv(path)
    series = []
    for row in rows:
        series.append(
            {
                "window_start_s": parse_float(row.get("window_start_s"), 0.0),
                "window_end_s": parse_float(row.get("window_end_s"), 0.0),
                "fov_on_time_bytes": parse_int(row.get("fov_on_time_bytes"), 0),
                "useful_goodput_kbps": parse_float(row.get("useful_goodput_kbps"), 0.0),
            }
        )
    return series


def aggregate_deadline_lateness(path):
    rows = load_csv(path)
    grouped = defaultdict(
        lambda: {
            "segment": 0,
            "total_tiles": 0,
            "missed_tiles": 0,
            "lateness_sum_ms": 0.0,
            "max_lateness_ms": 0.0,
        }
    )

    for row in rows:
        segment = parse_int(row.get("segment"))
        if segment is None:
            continue
        lateness_ms = parse_float(row.get("lateness_ms"), 0.0)
        missed = parse_bool(row.get("missed_deadline"))
        bucket = grouped[segment]
        bucket["segment"] = segment
        bucket["total_tiles"] += 1
        bucket["lateness_sum_ms"] += lateness_ms
        bucket["max_lateness_ms"] = max(bucket["max_lateness_ms"], lateness_ms)
        if missed:
            bucket["missed_tiles"] += 1

    series = []
    for segment in sorted(grouped):
        bucket = grouped[segment]
        total_tiles = bucket["total_tiles"] or 1
        series.append(
            {
                "segment": segment,
                "total_tiles": bucket["total_tiles"],
                "missed_tiles": bucket["missed_tiles"],
                "miss_rate_percent": 100.0 * bucket["missed_tiles"] / total_tiles,
                "avg_lateness_ms": bucket["lateness_sum_ms"] / total_tiles,
                "max_lateness_ms": bucket["max_lateness_ms"],
            }
        )
    return series


def classify_tile_outcome(row):
    if parse_bool(row.get("on_time")):
        return "on_time"
    if parse_bool(row.get("timedout")) and parse_bool(row.get("skipped")):
        return "timeout"
    return "late"


def aggregate_statistics(path):
    rows = load_csv(path)
    totals = {"on_time": 0, "late": 0, "timeout": 0, "total": 0}
    segments = defaultdict(
        lambda: {
            "segment": 0,
            "on_time": 0,
            "late": 0,
            "timeout": 0,
            "total": 0,
        }
    )

    for row in rows:
        segment = parse_int(row.get("segment"))
        if segment is None:
            continue
        outcome = classify_tile_outcome(row)
        totals[outcome] += 1
        totals["total"] += 1
        bucket = segments[segment]
        bucket["segment"] = segment
        bucket[outcome] += 1
        bucket["total"] += 1

    segment_series = []
    for segment in sorted(segments):
        bucket = segments[segment]
        total = bucket["total"] or 1
        segment_series.append(
            {
                "segment": segment,
                "on_time": bucket["on_time"],
                "late": bucket["late"],
                "timeout": bucket["timeout"],
                "total": bucket["total"],
                "on_time_rate_percent": 100.0 * bucket["on_time"] / total,
                "late_rate_percent": 100.0 * bucket["late"] / total,
                "timeout_rate_percent": 100.0 * bucket["timeout"] / total,
            }
        )

    return {"totals": totals, "by_segment": segment_series}


def parse_abr_decisions(stdout_path, first_segment):
    if not stdout_path or not os.path.exists(stdout_path):
        return []

    decisions = []
    pending_bola = None
    next_segment = first_segment or 1

    with open(stdout_path, encoding="utf-8", errors="replace") as handle:
        for raw_line in handle:
            line = raw_line.strip()
            bola_match = BOLA_ABR_RE.search(line)
            if bola_match:
                groups = bola_match.groupdict()
                decision = {
                    "segment": parse_int(groups["segment"], next_segment),
                    "cfg": groups["cfg"],
                    "fov_bitrate": parse_int(groups["fov_bitrate"], 0),
                    "nonfov_bitrate": parse_int(groups["nonfov_bitrate"], 0),
                    "avg_tp_kbps": parse_float(groups["avg_tp_kbps"], 0.0),
                    "buffer_s": parse_float(groups["buffer_s"], 0.0),
                    "budget_kb": parse_float(groups["budget_kb"], 0.0),
                    "size_kb": parse_float(groups["size_kb"], 0.0),
                    "score": parse_float(groups["score"], 0.0),
                    "guardrail": parse_bool(groups["guardrail"]),
                    "u_max": parse_float(groups["u_max"], 0.0),
                    "v_value": parse_float(groups["v_value"], 0.0),
                    "q_value": parse_float(groups["q_value"], 0.0),
                }
                decisions.append(decision)
                pending_bola = decision
                next_segment = max(next_segment, decision["segment"] + 1)
                continue

            generic_match = GENERIC_ABR_RE.search(line)
            if not generic_match:
                pending_bola = None
                continue

            groups = generic_match.groupdict()
            if pending_bola and pending_bola.get("cfg") == groups["cfg"]:
                pending_bola["fov_bitrate"] = parse_int(groups["fov_bitrate"], pending_bola["fov_bitrate"])
                pending_bola["nonfov_bitrate"] = parse_int(
                    groups["nonfov_bitrate"], pending_bola["nonfov_bitrate"]
                )
                pending_bola["avg_tp_kbps"] = parse_float(groups["avg_tp"], pending_bola["avg_tp_kbps"])
                pending_bola["buffer_s"] = parse_float(groups["buffer_s"], pending_bola["buffer_s"])
                pending_bola = None
                continue

            decision = {
                "segment": next_segment,
                "cfg": groups["cfg"],
                "fov_bitrate": parse_int(groups["fov_bitrate"], 0),
                "nonfov_bitrate": parse_int(groups["nonfov_bitrate"], 0),
                "avg_tp_kbps": parse_float(groups["avg_tp"], 0.0),
                "buffer_s": parse_float(groups["buffer_s"], 0.0),
                "budget_kb": None,
                "size_kb": None,
                "score": None,
                "guardrail": None,
                "u_max": None,
                "v_value": None,
                "q_value": None,
            }
            decisions.append(decision)
            next_segment += 1
            pending_bola = None

    for index, decision in enumerate(decisions, start=1):
        decision["decision_index"] = index

    return decisions


def normalize_wfq_utilization(path):
    rows = load_csv(path)
    normalized = []
    for row in rows:
        item = {}
        for key, value in row.items():
            item[key] = parse_float(value, 0.0) if value not in (None, "") else 0.0
        normalized.append(item)
    return normalized


def discover_runs(base_dir):
    runs = []
    if not os.path.isdir(base_dir):
        return runs

    for root, _dirs, _files in os.walk(base_dir):
        is_run, files = is_run_directory(root)
        if not is_run:
            continue

        meta = build_run_metadata(base_dir, root, files)
        fov_delivery = normalize_fov_delivery(files["fov_delivery"])
        statistics_rows = load_csv(files["statistics"])
        hint = first_segment_hint(fov_delivery, statistics_rows)

        run = {
            "id": meta["id"],
            "meta": meta,
            "files": {key: bool(value) for key, value in files.items()},
            "summary": normalize_summary(files["statistics_summary"]),
            "fov_delivery": fov_delivery,
            "fov_goodput": normalize_fov_goodput(files["fov_goodput"]),
            "deadline_by_segment": aggregate_deadline_lateness(files["deadline_lateness"]),
            "statistics": aggregate_statistics(files["statistics"]),
            "abr_decisions": parse_abr_decisions(files["stdout"], hint),
            "wfq_utilization": normalize_wfq_utilization(files["wfq_utilization"]),
        }
        runs.append(run)

    def run_sort_key(item):
        meta = item["meta"]
        return (
            str(meta.get("scenario") or ""),
            str(meta.get("server_mode") or ""),
            str(meta.get("abr_mode") or ""),
            parse_int(meta.get("base_latency_ms"), 0) or 0,
            item["id"],
        )

    return sorted(runs, key=run_sort_key)


def group_runs(runs):
    grouped = {}

    for run in runs:
        meta = run["meta"]
        key = "|".join(
            [
                str(meta.get("scenario") or ""),
                str(meta.get("server_mode") or ""),
                str(meta.get("base_latency_ms") or ""),
                str(meta.get("fov_mode") or "normal"),
            ]
        )
        group = grouped.setdefault(
            key,
            {
                "id": key,
                "scenario": meta.get("scenario"),
                "server_mode": meta.get("server_mode"),
                "base_latency_ms": meta.get("base_latency_ms"),
                "fov_mode": meta.get("fov_mode") or "normal",
                "runs_by_abr": defaultdict(list),
            },
        )
        group["runs_by_abr"][meta.get("abr_mode") or "unknown"].append(run["id"])

    ordered = {}
    for key in sorted(grouped):
        group = grouped[key]
        preferred = {}
        for abr_mode, run_ids in group["runs_by_abr"].items():
            preferred[abr_mode] = sorted(run_ids)[0]
            group["runs_by_abr"][abr_mode] = sorted(run_ids)
        group["preferred_runs"] = preferred
        group["has_pair"] = "bola" in preferred and "legacy" in preferred
        ordered[key] = group
    return ordered


def build_dataset(base_dir):
    runs = discover_runs(base_dir)
    data = {
        "base_dir": base_dir,
        "summary_metrics": SUMMARY_METRICS,
        "runs": {run["id"]: run for run in runs},
        "groups": group_runs(runs),
    }
    return data


HTML_TEMPLATE = r'''<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8" />
  <title>ABR Validation Dashboard</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Segoe UI", system-ui, sans-serif;
      background: #08111f;
      color: #e5eef8;
    }
    .page {
      max-width: 1400px;
      margin: 0 auto;
      padding: 20px 24px 40px;
    }
    header {
      margin-bottom: 18px;
      border-bottom: 1px solid #20324a;
      padding-bottom: 14px;
    }
    h1 {
      margin: 0;
      font-size: 28px;
      font-weight: 700;
    }
    .subtitle {
      margin-top: 6px;
      color: #93acc8;
      max-width: 900px;
      line-height: 1.5;
      font-size: 14px;
    }
    .tabs {
      display: flex;
      gap: 8px;
      margin: 18px 0 22px;
    }
    .tab-btn {
      border: 1px solid #28405f;
      background: #102036;
      color: #d7e4f2;
      padding: 10px 14px;
      border-radius: 999px;
      cursor: pointer;
      font-size: 14px;
    }
    .tab-btn.active {
      background: #d7e4f2;
      color: #08111f;
      border-color: #d7e4f2;
    }
    .tab-panel { display: none; }
    .tab-panel.active { display: block; }
    .controls {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin-bottom: 18px;
    }
    .control {
      min-width: 180px;
    }
    .control label {
      display: block;
      margin-bottom: 6px;
      font-size: 12px;
      color: #7f97b5;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    select {
      width: 100%;
      background: #102036;
      color: #e5eef8;
      border: 1px solid #28405f;
      border-radius: 10px;
      padding: 9px 12px;
      font-size: 14px;
    }
    .panel {
      background: #0f1c30;
      border: 1px solid #1f324d;
      border-radius: 16px;
      padding: 16px;
      margin-bottom: 18px;
    }
    .panel h2, .panel h3 {
      margin: 0 0 6px;
      font-size: 16px;
    }
    .panel .desc {
      margin: 0 0 14px;
      color: #8ea6c3;
      font-size: 13px;
      line-height: 1.45;
    }
    .meta-grid, .metric-grid, .summary-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 12px;
    }
    .meta-card, .metric-card, .summary-card {
      background: #14243b;
      border: 1px solid #223753;
      border-radius: 14px;
      padding: 14px;
      min-height: 98px;
    }
    .meta-card .label, .metric-card .label, .summary-card .label {
      color: #8ea6c3;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      margin-bottom: 8px;
    }
    .meta-card .value {
      font-size: 19px;
      font-weight: 700;
    }
    .meta-card .detail {
      margin-top: 8px;
      color: #9db2cb;
      font-size: 12px;
      line-height: 1.4;
    }
    .metric-card .metric-name {
      font-size: 14px;
      font-weight: 600;
      margin-bottom: 8px;
    }
    .metric-values {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 8px;
      margin-bottom: 8px;
    }
    .metric-values div {
      background: #102036;
      border-radius: 10px;
      padding: 8px 10px;
      border: 1px solid #223753;
    }
    .metric-values .name {
      color: #8ea6c3;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .metric-values .val {
      font-size: 18px;
      font-weight: 700;
      margin-top: 4px;
    }
    .delta {
      font-size: 12px;
      font-weight: 600;
    }
    .delta.good { color: #57d18c; }
    .delta.bad { color: #ff8a8a; }
    .delta.neutral { color: #8ea6c3; }
    .chart-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
      gap: 16px;
      margin-top: 12px;
    }
    .chart-host {
      width: 100%;
      min-height: 280px;
    }
    .chart-host.small {
      min-height: 240px;
    }
    .chart-legend {
      display: flex;
      flex-wrap: wrap;
      gap: 14px;
      margin-top: 10px;
      color: #9db2cb;
      font-size: 12px;
    }
    .legend-item {
      display: inline-flex;
      align-items: center;
      gap: 8px;
    }
    .swatch {
      width: 12px;
      height: 12px;
      border-radius: 999px;
    }
    .empty {
      color: #8ea6c3;
      font-size: 14px;
      line-height: 1.5;
      padding: 24px 8px;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 12px;
    }
    th, td {
      border-bottom: 1px solid #20324a;
      padding: 8px 10px;
      text-align: left;
      vertical-align: top;
    }
    th {
      color: #8ea6c3;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      font-size: 11px;
    }
    tbody tr:hover {
      background: rgba(255, 255, 255, 0.02);
    }
    .two-col {
      display: grid;
      grid-template-columns: 1.5fr 1fr;
      gap: 16px;
    }
    @media (max-width: 980px) {
      .two-col,
      .chart-grid {
        grid-template-columns: 1fr;
      }
    }
  </style>
</head>
<body>
  <div class="page">
    <header>
      <h1>ABR validation dashboard</h1>
      <div class="subtitle">
        This dashboard combines client metrics, optional ABR decision parsing from stdout, and WFQ telemetry
        so you can validate whether the ABR is making coherent choices and whether those choices improve
        FoV delivery, useful goodput, deadline misses, and timely delivery.
      </div>
    </header>

    <div class="tabs">
      <button class="tab-btn" id="tab-btn-compare">ABR comparison</button>
      <button class="tab-btn" id="tab-btn-inspector">Run inspector</button>
    </div>

    <section class="tab-panel" id="tab-compare">
      <div class="controls">
        <div class="control">
          <label>Scenario</label>
          <select id="compare-scenario"></select>
        </div>
        <div class="control">
          <label>Scheduler</label>
          <select id="compare-scheduler"></select>
        </div>
        <div class="control">
          <label>Base latency</label>
          <select id="compare-baselatency"></select>
        </div>
        <div class="control">
          <label>FoV</label>
          <select id="compare-fov"></select>
        </div>
      </div>

      <div id="compare-content"></div>
    </section>

    <section class="tab-panel" id="tab-inspector">
      <div class="controls">
        <div class="control" style="min-width: 320px;">
          <label>Run</label>
          <select id="inspector-run"></select>
        </div>
      </div>
      <div id="inspector-content"></div>
    </section>
  </div>

  <script>
    const DATA = __DATA_JSON__;
    const COLORS = {
      bola: '#38bdf8',
      legacy: '#fb7185',
      on_time: '#4ade80',
      late: '#f59e0b',
      timeout: '#ef4444',
      fov: '#38bdf8',
      nonfov: '#f59e0b',
      throughput: '#a78bfa',
      buffer: '#4ade80',
      wfq_low: '#22c55e',
      wfq_medium: '#eab308',
      wfq_high: '#f97316'
    };

    function byId(id) {
      return document.getElementById(id);
    }

    function fmtNumber(value, digits = 2) {
      if (value === null || value === undefined || value === '' || Number.isNaN(Number(value))) {
        return 'N/A';
      }
      return Number(value).toFixed(digits);
    }

    function fmtMetric(value, unit) {
      if (value === null || value === undefined || value === '' || Number.isNaN(Number(value))) {
        return 'N/A';
      }
      const n = Number(value);
      if (unit === '%') return `${n.toFixed(2)}%`;
      if (unit === 'kbps') return `${n.toFixed(2)} kbps`;
      return n.toFixed(2);
    }

    function humanScenario(meta) {
      if (meta.scenario === undefined || meta.scenario === null || meta.scenario === '') {
        if (meta.loss_pct !== undefined && meta.delay_ms !== undefined) {
          return `loss ${meta.loss_pct}% / delay ${meta.delay_ms} ms`;
        }
        return 'custom';
      }
      return String(meta.scenario).startsWith('net') ? String(meta.scenario) : `scenario ${meta.scenario}`;
    }

    function humanRunLabel(run) {
      const m = run.meta || {};
      const baseLatency = m.base_latency_ms !== undefined && m.base_latency_ms !== null ? `${m.base_latency_ms} ms` : 'n/a';
      return `${run.id} - ${m.server_mode || 'unknown'} - ${m.abr_mode || 'unknown'} - ${humanScenario(m)} - base ${baseLatency}`;
    }

    function allRuns() {
      return Object.values(DATA.runs || {});
    }

    function compareGroups() {
      return Object.values(DATA.groups || {}).filter(group => group.has_pair);
    }

    function optionValues(items, mapper) {
      const values = [];
      const seen = new Set();
      items.forEach(item => {
        const value = mapper(item);
        if (seen.has(value)) return;
        seen.add(value);
        values.push(value);
      });
      return values;
    }

    function fillSelect(selectId, values, formatter, currentValue) {
      const select = byId(selectId);
      const options = ['all'].concat(values);
      select.innerHTML = options.map(value => {
        const selected = value === currentValue ? 'selected' : '';
        return `<option value="${value}" ${selected}>${formatter(value)}</option>`;
      }).join('');
    }

    function filtersFromUI() {
      return {
        scenario: byId('compare-scenario').value,
        scheduler: byId('compare-scheduler').value,
        baseLatency: byId('compare-baselatency').value,
        fov: byId('compare-fov').value,
      };
    }

    function groupMatches(group, filters) {
      const checks = [
        ['scenario', group.scenario === undefined || group.scenario === null || group.scenario === '' ? 'custom' : String(group.scenario)],
        ['scheduler', group.server_mode || 'unknown'],
        ['baseLatency', group.base_latency_ms === undefined || group.base_latency_ms === null ? 'unknown' : String(group.base_latency_ms)],
        ['fov', group.fov_mode || 'normal'],
      ];
      return checks.every(([key, value]) => filters[key] === 'all' || filters[key] === value);
    }

    function getSelectedCompareGroup() {
      const groups = compareGroups();
      const filters = filtersFromUI();
      return groups.find(group => groupMatches(group, filters)) || null;
    }

    function metricDeltaClass(metric, bolaValue, legacyValue) {
      if (bolaValue === null || legacyValue === null) return 'neutral';
      const diff = Number(bolaValue) - Number(legacyValue);
      if (Math.abs(diff) < 1e-9) return 'neutral';
      if (metric.better === 'higher') return diff > 0 ? 'good' : 'bad';
      return diff < 0 ? 'good' : 'bad';
    }

    function buildCompareMeta(group, bolaRun, legacyRun) {
      const fov = group.fov_mode || 'normal';
      return `
        <div class="panel">
          <h2>Comparison context</h2>
          <div class="meta-grid">
            <div class="meta-card">
              <div class="label">Scenario</div>
              <div class="value">${humanScenario(group)}</div>
              <div class="detail">Pair available for BOLA and legacy.</div>
            </div>
            <div class="meta-card">
              <div class="label">Scheduler</div>
              <div class="value">${group.server_mode || 'unknown'}</div>
              <div class="detail">Comparison key uses scenario + scheduler + base latency + FoV.</div>
            </div>
            <div class="meta-card">
              <div class="label">Base latency</div>
              <div class="value">${group.base_latency_ms !== undefined && group.base_latency_ms !== null ? `${group.base_latency_ms} ms` : 'N/A'}</div>
              <div class="detail">FoV mode: ${fov}</div>
            </div>
            <div class="meta-card">
              <div class="label">Runs</div>
              <div class="value">${bolaRun.id}</div>
              <div class="detail">Legacy: ${legacyRun.id}</div>
            </div>
          </div>
        </div>
      `;
    }

    function buildCompareMetricCards(bolaRun, legacyRun) {
      const cards = (DATA.summary_metrics || []).map(metric => {
        const bolaValue = bolaRun.summary ? bolaRun.summary[metric.key] : null;
        const legacyValue = legacyRun.summary ? legacyRun.summary[metric.key] : null;
        const deltaClass = metricDeltaClass(metric, bolaValue, legacyValue);
        let deltaText = 'Delta unavailable';
        if (bolaValue !== null && bolaValue !== undefined && legacyValue !== null && legacyValue !== undefined) {
          const diff = Number(bolaValue) - Number(legacyValue);
          const prefix = diff > 0 ? '+' : '';
          deltaText = `BOLA - legacy: ${prefix}${Number(diff).toFixed(2)}${metric.unit === '%' ? '%' : metric.unit === 'kbps' ? ' kbps' : ''}`;
        }
        return `
          <div class="metric-card">
            <div class="label">Summary metric</div>
            <div class="metric-name">${metric.label}</div>
            <div class="metric-values">
              <div>
                <div class="name">BOLA</div>
                <div class="val">${fmtMetric(bolaValue, metric.unit)}</div>
              </div>
              <div>
                <div class="name">Legacy</div>
                <div class="val">${fmtMetric(legacyValue, metric.unit)}</div>
              </div>
            </div>
            <div class="delta ${deltaClass}">${deltaText}</div>
          </div>
        `;
      }).join('');

      return `
        <div class="panel">
          <h2>Side-by-side summary metrics</h2>
          <p class="desc">Use these cards to decide whether BOLA improved FoV delivery and deadline behavior without increasing stale or late data.</p>
          <div class="metric-grid">${cards}</div>
        </div>
      `;
    }

    function renderSvgLineChart(targetId, title, description, datasets, options = {}) {
      const target = byId(targetId);
      const cleaned = datasets.filter(set => (set.points || []).length);
      if (!cleaned.length) {
        target.innerHTML = `<div class="panel"><h3>${title}</h3><p class="desc">${description}</p><div class="empty">No data available for this chart.</div></div>`;
        return;
      }

      const width = 560;
      const height = 280;
      const pad = { left: 48, right: 18, top: 18, bottom: 34 };
      const allPoints = cleaned.flatMap(set => set.points);
      const xs = allPoints.map(point => Number(point.x));
      const ys = allPoints.map(point => Number(point.y));
      let minX = Math.min(...xs);
      let maxX = Math.max(...xs);
      let minY = options.minY !== undefined ? options.minY : Math.min(...ys);
      let maxY = options.maxY !== undefined ? options.maxY : Math.max(...ys);
      if (minX === maxX) maxX += 1;
      if (minY === maxY) {
        minY = options.minY !== undefined ? options.minY : 0;
        maxY += 1;
      }

      const plotWidth = width - pad.left - pad.right;
      const plotHeight = height - pad.top - pad.bottom;
      const scaleX = value => pad.left + ((value - minX) / (maxX - minX)) * plotWidth;
      const scaleY = value => pad.top + plotHeight - ((value - minY) / (maxY - minY)) * plotHeight;

      const gridLines = [];
      for (let i = 0; i <= 4; i++) {
        const ratio = i / 4;
        const y = pad.top + ratio * plotHeight;
        const value = maxY - ratio * (maxY - minY);
        gridLines.push(`
          <line x1="${pad.left}" y1="${y}" x2="${width - pad.right}" y2="${y}" stroke="#233954" stroke-dasharray="4 4" />
          <text x="6" y="${y + 4}" fill="#8ea6c3" font-size="11">${(options.yFormatter || (v => Number(v).toFixed(1)))(value)}</text>
        `);
      }

      const lines = cleaned.map(set => {
        const polyline = set.points.map(point => `${scaleX(Number(point.x))},${scaleY(Number(point.y))}`).join(' ');
        const circles = set.points.map(point => {
          const x = scaleX(Number(point.x));
          const y = scaleY(Number(point.y));
          return `<circle cx="${x}" cy="${y}" r="3.5" fill="${set.color}" />`;
        }).join('');
        return `<polyline fill="none" stroke="${set.color}" stroke-width="2.5" points="${polyline}" />${circles}`;
      }).join('');

      const legend = cleaned.map(set => `
        <span class="legend-item"><span class="swatch" style="background:${set.color};"></span>${set.label}</span>
      `).join('');

      target.innerHTML = `
        <div class="panel">
          <h3>${title}</h3>
          <p class="desc">${description}</p>
          <div class="chart-host">
            <svg viewBox="0 0 ${width} ${height}" width="100%" height="100%" role="img" aria-label="${title}">
              ${gridLines.join('')}
              <line x1="${pad.left}" y1="${pad.top}" x2="${pad.left}" y2="${height - pad.bottom}" stroke="#35506e" />
              <line x1="${pad.left}" y1="${height - pad.bottom}" x2="${width - pad.right}" y2="${height - pad.bottom}" stroke="#35506e" />
              ${lines}
            </svg>
          </div>
          <div class="chart-legend">${legend}</div>
        </div>
      `;
    }

    function renderStackedOutcomeChart(targetId, title, description, rows) {
      const target = byId(targetId);
      if (!rows.length) {
        target.innerHTML = `<div class="panel"><h3>${title}</h3><p class="desc">${description}</p><div class="empty">No per-tile client statistics found.</div></div>`;
        return;
      }

      const width = 560;
      const height = 280;
      const pad = { left: 48, right: 18, top: 18, bottom: 34 };
      const plotWidth = width - pad.left - pad.right;
      const plotHeight = height - pad.top - pad.bottom;
      const maxTotal = Math.max(...rows.map(row => row.total || 0), 1);
      const barWidth = plotWidth / Math.max(rows.length, 1) * 0.7;

      const bars = rows.map((row, index) => {
        const x = pad.left + (index + 0.15) * (plotWidth / rows.length);
        const stacks = [
          { key: 'on_time', color: COLORS.on_time },
          { key: 'late', color: COLORS.late },
          { key: 'timeout', color: COLORS.timeout },
        ];
        let offset = 0;
        const rects = stacks.map(stack => {
          const value = row[stack.key] || 0;
          const h = (value / maxTotal) * plotHeight;
          const y = pad.top + plotHeight - offset - h;
          offset += h;
          return `<rect x="${x}" y="${y}" width="${barWidth}" height="${Math.max(h, 0)}" fill="${stack.color}" />`;
        }).join('');
        const labelX = x + barWidth / 2;
        return `${rects}<text x="${labelX}" y="${height - 10}" text-anchor="middle" fill="#8ea6c3" font-size="10">${row.segment}</text>`;
      }).join('');

      target.innerHTML = `
        <div class="panel">
          <h3>${title}</h3>
          <p class="desc">${description}</p>
          <div class="chart-host">
            <svg viewBox="0 0 ${width} ${height}" width="100%" height="100%" role="img" aria-label="${title}">
              <line x1="${pad.left}" y1="${pad.top}" x2="${pad.left}" y2="${height - pad.bottom}" stroke="#35506e" />
              <line x1="${pad.left}" y1="${height - pad.bottom}" x2="${width - pad.right}" y2="${height - pad.bottom}" stroke="#35506e" />
              ${bars}
            </svg>
          </div>
          <div class="chart-legend">
            <span class="legend-item"><span class="swatch" style="background:${COLORS.on_time};"></span>On time</span>
            <span class="legend-item"><span class="swatch" style="background:${COLORS.late};"></span>Late response</span>
            <span class="legend-item"><span class="swatch" style="background:${COLORS.timeout};"></span>Timeout</span>
          </div>
        </div>
      `;
    }

    function wfqWeightValue(row, rawKey, normKey) {
      const raw = Number(row[rawKey] || 0);
      if (raw > 0) return raw;
      const norm = Number(row[normKey] || 0);
      return norm > 0 ? norm * 6.0 : 0;
    }

    function drawWfqShare(canvasId, rows) {
      const canvas = byId(canvasId);
      const ctx = canvas.getContext('2d');
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      if (!rows.length) return;

      const pad = { left: 42, right: 18, top: 16, bottom: 28 };
      const plotWidth = canvas.width - pad.left - pad.right;
      const plotHeight = canvas.height - pad.top - pad.bottom;
      const xAt = index => pad.left + (index / Math.max(rows.length - 1, 1)) * plotWidth;
      const yAt = value => pad.top + plotHeight - value * plotHeight;

      ctx.strokeStyle = '#35506e';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(pad.left, pad.top);
      ctx.lineTo(pad.left, canvas.height - pad.bottom);
      ctx.lineTo(canvas.width - pad.right, canvas.height - pad.bottom);
      ctx.stroke();

      const colors = [COLORS.wfq_low, COLORS.wfq_medium, COLORS.wfq_high];

      function shareTriplet(row) {
        let low = Number(row.share_low || 0);
        let medium = Number(row.share_medium || 0);
        let high = Number(row.share_high || 0);
        const sum = low + medium + high;
        if (sum > 1.5) {
          low /= 100;
          medium /= 100;
          high /= 100;
        }
        const normalized = low + medium + high;
        if (normalized <= 0) return [0, 0, 0];
        return [low / normalized, medium / normalized, high / normalized];
      }

      for (let band = 0; band < 3; band++) {
        ctx.beginPath();
        ctx.fillStyle = colors[band];
        for (let i = 0; i < rows.length; i++) {
          const [low, medium, high] = shareTriplet(rows[i]);
          const values = [low, medium, high];
          const bottom = values.slice(0, band).reduce((acc, value) => acc + value, 0);
          const top = bottom + values[band];
          const x = xAt(i);
          const yTop = yAt(top);
          const yBottom = yAt(bottom);
          if (i === 0) {
            ctx.moveTo(x, yBottom);
            ctx.lineTo(x, yTop);
          } else {
            ctx.lineTo(x, yTop);
          }
        }
        for (let i = rows.length - 1; i >= 0; i--) {
          const [low, medium, high] = shareTriplet(rows[i]);
          const values = [low, medium, high];
          const bottom = values.slice(0, band).reduce((acc, value) => acc + value, 0);
          ctx.lineTo(xAt(i), yAt(bottom));
        }
        ctx.closePath();
        ctx.globalAlpha = 0.85;
        ctx.fill();
        ctx.globalAlpha = 1;
      }
    }

    function drawWfqWeights(canvasId, rows) {
      const canvas = byId(canvasId);
      const ctx = canvas.getContext('2d');
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      if (!rows.length) return;

      const pad = { left: 42, right: 18, top: 16, bottom: 28 };
      const plotWidth = canvas.width - pad.left - pad.right;
      const plotHeight = canvas.height - pad.top - pad.bottom;
      const xAt = index => pad.left + (index / Math.max(rows.length - 1, 1)) * plotWidth;
      const values = rows.flatMap(row => [
        wfqWeightValue(row, 'raw_w_low', 'w_low'),
        wfqWeightValue(row, 'raw_w_medium', 'w_medium'),
        wfqWeightValue(row, 'raw_w_high', 'w_high')
      ]);
      let minY = Math.min(...values);
      let maxY = Math.max(...values);
      if (minY === maxY) maxY += 1;
      const yAt = value => pad.top + plotHeight - ((value - minY) / (maxY - minY)) * plotHeight;

      ctx.strokeStyle = '#35506e';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(pad.left, pad.top);
      ctx.lineTo(pad.left, canvas.height - pad.bottom);
      ctx.lineTo(canvas.width - pad.right, canvas.height - pad.bottom);
      ctx.stroke();

      const series = [
        { raw: 'raw_w_low', norm: 'w_low', color: COLORS.wfq_low },
        { raw: 'raw_w_medium', norm: 'w_medium', color: COLORS.wfq_medium },
        { raw: 'raw_w_high', norm: 'w_high', color: COLORS.wfq_high }
      ];

      series.forEach(set => {
        ctx.beginPath();
        ctx.strokeStyle = set.color;
        ctx.lineWidth = 2.5;
        rows.forEach((row, index) => {
          const x = xAt(index);
          const y = yAt(wfqWeightValue(row, set.raw, set.norm));
          if (index === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        });
        ctx.stroke();
      });
    }

    function compareChartSeries(run, sourceKey, xKey, yKey) {
      return (run[sourceKey] || []).map(item => ({ x: Number(item[xKey]), y: Number(item[yKey]) }));
    }

    function renderCompareView() {
      const container = byId('compare-content');
      const groups = compareGroups();
      if (!groups.length) {
        container.innerHTML = `
          <div class="panel">
            <h2>No ABR pair found</h2>
            <div class="empty">
              This base directory does not contain a complete BOLA vs legacy pair under the same
              scenario + scheduler + base latency + FoV key yet.
            </div>
          </div>
        `;
        return;
      }

      const group = getSelectedCompareGroup();
      if (!group) {
        container.innerHTML = '<div class="panel"><div class="empty">No group matches the current filters.</div></div>';
        return;
      }

      const bolaRun = DATA.runs[group.preferred_runs.bola];
      const legacyRun = DATA.runs[group.preferred_runs.legacy];
      container.innerHTML = `
        ${buildCompareMeta(group, bolaRun, legacyRun)}
        ${buildCompareMetricCards(bolaRun, legacyRun)}
        <div class="chart-grid">
          <div id="compare-fov-hit"></div>
          <div id="compare-goodput"></div>
          <div id="compare-deadline-miss"></div>
          <div id="compare-avg-lateness"></div>
        </div>
      `;

      renderSvgLineChart(
        'compare-fov-hit',
        'FoV hit rate per segment',
        'This is the most direct visual signal for whether the ABR preserved useful FoV delivery while the network changed.',
        [
          { label: 'BOLA', color: COLORS.bola, points: compareChartSeries(bolaRun, 'fov_delivery', 'segment', 'fov_hit_rate_percent') },
          { label: 'Legacy', color: COLORS.legacy, points: compareChartSeries(legacyRun, 'fov_delivery', 'segment', 'fov_hit_rate_percent') }
        ],
        { minY: 0, maxY: 100, yFormatter: value => `${Number(value).toFixed(0)}%` }
      );

      renderSvgLineChart(
        'compare-goodput',
        'Useful FoV goodput by window',
        'If an ABR chooses better rates, useful FoV goodput should improve without collapsing deadline behavior.',
        [
          { label: 'BOLA', color: COLORS.bola, points: compareChartSeries(bolaRun, 'fov_goodput', 'window_end_s', 'useful_goodput_kbps') },
          { label: 'Legacy', color: COLORS.legacy, points: compareChartSeries(legacyRun, 'fov_goodput', 'window_end_s', 'useful_goodput_kbps') }
        ],
        { minY: 0, yFormatter: value => `${Number(value).toFixed(0)}` }
      );

      renderSvgLineChart(
        'compare-deadline-miss',
        'Deadline miss rate by segment',
        'This aggregates client deadline-lateness samples per segment, which helps separate a smart ABR from one that only increases bitrate.',
        [
          { label: 'BOLA', color: COLORS.bola, points: compareChartSeries(bolaRun, 'deadline_by_segment', 'segment', 'miss_rate_percent') },
          { label: 'Legacy', color: COLORS.legacy, points: compareChartSeries(legacyRun, 'deadline_by_segment', 'segment', 'miss_rate_percent') }
        ],
        { minY: 0, maxY: 100, yFormatter: value => `${Number(value).toFixed(0)}%` }
      );

      renderSvgLineChart(
        'compare-avg-lateness',
        'Average lateness by segment',
        'Higher values here indicate the client kept receiving data after the segment deadline, which often means the ABR asked for too much.',
        [
          { label: 'BOLA', color: COLORS.bola, points: compareChartSeries(bolaRun, 'deadline_by_segment', 'segment', 'avg_lateness_ms') },
          { label: 'Legacy', color: COLORS.legacy, points: compareChartSeries(legacyRun, 'deadline_by_segment', 'segment', 'avg_lateness_ms') }
        ],
        { minY: 0, yFormatter: value => `${Number(value).toFixed(0)} ms` }
      );
    }

    function buildSummaryCards(run) {
      const cards = (DATA.summary_metrics || []).map(metric => `
        <div class="summary-card">
          <div class="label">${metric.label}</div>
          <div style="font-size:24px;font-weight:700;">${fmtMetric(run.summary ? run.summary[metric.key] : null, metric.unit)}</div>
          <div style="margin-top:8px;color:#8ea6c3;font-size:12px;">${metric.better === 'higher' ? 'Higher is better' : 'Lower is better'}</div>
        </div>
      `).join('');

      return `
        <div class="panel">
          <h2>Client summary metrics</h2>
          <p class="desc">These values come from statistics-summary and summarize the final effect of the ABR and scheduler together.</p>
          <div class="summary-grid">${cards}</div>
        </div>
      `;
    }

    function buildInspectorMeta(run) {
      const meta = run.meta || {};
      return `
        <div class="panel">
          <h2>Run context</h2>
          <div class="meta-grid">
            <div class="meta-card">
              <div class="label">Run id</div>
              <div class="value">${run.id}</div>
              <div class="detail">${meta.meta_partial ? 'Metadata partially inferred from path/name.' : 'Metadata loaded from experiment.env.'}</div>
            </div>
            <div class="meta-card">
              <div class="label">ABR</div>
              <div class="value">${meta.abr_mode || 'unknown'}</div>
              <div class="detail">Scheduler: ${meta.server_mode || 'unknown'}</div>
            </div>
            <div class="meta-card">
              <div class="label">Scenario</div>
              <div class="value">${humanScenario(meta)}</div>
              <div class="detail">FoV mode: ${meta.fov_mode || 'normal'}</div>
            </div>
            <div class="meta-card">
              <div class="label">Network</div>
              <div class="value">${meta.server_bw_mbps !== undefined && meta.server_bw_mbps !== null ? `${meta.server_bw_mbps}/${meta.client_bw_mbps || meta.server_bw_mbps} Mbps` : 'N/A'}</div>
              <div class="detail">loss ${meta.loss_pct !== undefined ? meta.loss_pct : 'N/A'}% - delay ${meta.delay_ms !== undefined ? meta.delay_ms : 'N/A'} ms - base latency ${meta.base_latency_ms !== undefined ? meta.base_latency_ms : 'N/A'} ms</div>
            </div>
          </div>
        </div>
      `;
    }

    function buildOutcomeTotals(run) {
      const totals = (run.statistics && run.statistics.totals) || { on_time: 0, late: 0, timeout: 0, total: 0 };
      return `
        <div class="panel">
          <h2>Tile outcome totals</h2>
          <p class="desc">These counts come from per-tile client statistics and help show whether the ABR produced mostly on-time tiles, late responses, or hard timeouts.</p>
          <div class="summary-grid">
            <div class="summary-card"><div class="label">On time</div><div style="font-size:24px;font-weight:700;">${totals.on_time}</div></div>
            <div class="summary-card"><div class="label">Late</div><div style="font-size:24px;font-weight:700;">${totals.late}</div></div>
            <div class="summary-card"><div class="label">Timeout</div><div style="font-size:24px;font-weight:700;">${totals.timeout}</div></div>
          </div>
        </div>
      `;
    }

    function buildDecisionsTable(run) {
      const decisions = run.abr_decisions || [];
      if (!decisions.length) {
        return `
          <div class="panel">
            <h2>ABR decisions</h2>
            <div class="empty">No parseable ABR decision lines were found in stdout for this run.</div>
          </div>
        `;
      }

      const rows = decisions.slice(0, 40).map(decision => `
        <tr>
          <td>${decision.segment}</td>
          <td>${decision.cfg || 'N/A'}</td>
          <td>${decision.fov_bitrate}</td>
          <td>${decision.nonfov_bitrate}</td>
          <td>${fmtNumber(decision.avg_tp_kbps, 2)}</td>
          <td>${fmtNumber(decision.buffer_s, 2)}</td>
          <td>${decision.guardrail === null || decision.guardrail === undefined ? 'N/A' : decision.guardrail}</td>
          <td>${decision.score === null || decision.score === undefined ? 'N/A' : fmtNumber(decision.score, 4)}</td>
        </tr>
      `).join('');

      return `
        <div class="panel">
          <h2>ABR decision samples</h2>
          <p class="desc">Parsed from stdout when available. This is the direct evidence of what the ABR chose segment by segment.</p>
          <table>
            <thead>
              <tr>
                <th>Segment</th>
                <th>Cfg</th>
                <th>FoV bitrate</th>
                <th>Non-FoV bitrate</th>
                <th>Avg tp (kbps)</th>
                <th>Buffer (s)</th>
                <th>Guardrail</th>
                <th>Score</th>
              </tr>
            </thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      `;
    }

    function buildSummaryTable(run) {
      const keys = Object.keys(run.summary || {});
      if (!keys.length) {
        return '<div class="panel"><h2>Raw summary</h2><div class="empty">No statistics-summary file found for this run.</div></div>';
      }
      const rows = keys.map(key => `
        <tr>
          <td>${key}</td>
          <td>${fmtNumber(run.summary[key], 2)}</td>
        </tr>
      `).join('');
      return `
        <div class="panel">
          <h2>Raw summary values</h2>
          <table>
            <thead><tr><th>Metric</th><th>Value</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      `;
    }

    function renderInspectorView() {
      const select = byId('inspector-run');
      const run = DATA.runs[select.value];
      const container = byId('inspector-content');
      if (!run) {
        container.innerHTML = '<div class="panel"><div class="empty">No run selected.</div></div>';
        return;
      }

      container.innerHTML = `
        ${buildInspectorMeta(run)}
        ${buildSummaryCards(run)}
        <div class="chart-grid">
          <div id="inspector-bitrate"></div>
          <div id="inspector-throughput"></div>
          <div id="inspector-buffer"></div>
          <div id="inspector-fov-hit"></div>
          <div id="inspector-goodput"></div>
          <div id="inspector-deadline"></div>
        </div>
        ${buildOutcomeTotals(run)}
        <div id="inspector-outcomes"></div>
        <div id="inspector-wfq"></div>
        <div class="two-col">
          ${buildDecisionsTable(run)}
          ${buildSummaryTable(run)}
        </div>
      `;

      const decisions = run.abr_decisions || [];
      renderSvgLineChart(
        'inspector-bitrate',
        'ABR bitrate decisions by segment',
        'Check if bitrate choices move up or down coherently as throughput and buffer change.',
        [
          {
            label: 'FoV bitrate',
            color: COLORS.fov,
            points: decisions.map(item => ({ x: item.segment, y: item.fov_bitrate }))
          },
          {
            label: 'Non-FoV bitrate',
            color: COLORS.nonfov,
            points: decisions.map(item => ({ x: item.segment, y: item.nonfov_bitrate }))
          }
        ],
        { minY: 0, yFormatter: value => Number(value).toFixed(0) }
      );

      renderSvgLineChart(
        'inspector-throughput',
        'Observed throughput per decision',
        'Parsed from stdout when available. This should explain why the ABR changed or kept a given config.',
        [
          {
            label: 'Avg throughput',
            color: COLORS.throughput,
            points: decisions.map(item => ({ x: item.segment, y: item.avg_tp_kbps }))
          }
        ],
        { minY: 0, yFormatter: value => `${Number(value).toFixed(0)}` }
      );

      renderSvgLineChart(
        'inspector-buffer',
        'Observed buffer per decision',
        'If buffer stays low while bitrate stays high, the ABR is likely being too aggressive.',
        [
          {
            label: 'Buffer',
            color: COLORS.buffer,
            points: decisions.map(item => ({ x: item.segment, y: item.buffer_s }))
          }
        ],
        { minY: 0, yFormatter: value => `${Number(value).toFixed(1)} s` }
      );

      renderSvgLineChart(
        'inspector-fov-hit',
        'FoV hit rate per segment',
        'Final client-side effect by segment.',
        [
          {
            label: 'FoV hit rate',
            color: COLORS.fov,
            points: (run.fov_delivery || []).map(item => ({ x: item.segment, y: item.fov_hit_rate_percent }))
          }
        ],
        { minY: 0, maxY: 100, yFormatter: value => `${Number(value).toFixed(0)}%` }
      );

      renderSvgLineChart(
        'inspector-goodput',
        'Useful FoV goodput by window',
        'Useful goodput is a better signal than raw throughput because it only counts on-time FoV bytes.',
        [
          {
            label: 'FoV useful goodput',
            color: COLORS.fov,
            points: (run.fov_goodput || []).map(item => ({ x: item.window_end_s, y: item.useful_goodput_kbps }))
          }
        ],
        { minY: 0, yFormatter: value => `${Number(value).toFixed(0)}` }
      );

      renderSvgLineChart(
        'inspector-deadline',
        'Deadline miss rate by segment',
        'This chart helps separate network pressure from ABR overreach.',
        [
          {
            label: 'Miss rate',
            color: COLORS.timeout,
            points: (run.deadline_by_segment || []).map(item => ({ x: item.segment, y: item.miss_rate_percent }))
          }
        ],
        { minY: 0, maxY: 100, yFormatter: value => `${Number(value).toFixed(0)}%` }
      );

      renderStackedOutcomeChart(
        'inspector-outcomes',
        'Per-segment tile outcomes',
        'This uses statistics-*.csv to show how many tiles were on time, late, or timed out in each segment.',
        (run.statistics && run.statistics.by_segment) || []
      );

      const wfqHost = byId('inspector-wfq');
      const shouldShowWfq = (run.meta && run.meta.server_mode === 'wfq') && (run.wfq_utilization || []).length;
      if (!shouldShowWfq) {
        wfqHost.innerHTML = '';
        return;
      }
      wfqHost.innerHTML = `
        <div class="panel">
          <h2>WFQ telemetry</h2>
          <p class="desc">These charts are shown only for WFQ runs and help explain whether scheduler adaptation and ABR behavior were aligned.</p>
          <div class="chart-grid">
            <div>
              <canvas id="wfq-share-canvas" width="560" height="260"></canvas>
              <div class="chart-legend">
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_low};"></span>LOW share</span>
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_medium};"></span>MED share</span>
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_high};"></span>HIGH share</span>
              </div>
            </div>
            <div>
              <canvas id="wfq-weight-canvas" width="560" height="260"></canvas>
              <div class="chart-legend">
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_low};"></span>W low</span>
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_medium};"></span>W medium</span>
                <span class="legend-item"><span class="swatch" style="background:${COLORS.wfq_high};"></span>W high</span>
              </div>
            </div>
          </div>
        </div>
      `;
      drawWfqShare('wfq-share-canvas', run.wfq_utilization || []);
      drawWfqWeights('wfq-weight-canvas', run.wfq_utilization || []);
    }

    function setActiveTab(name) {
      ['compare', 'inspector'].forEach(tab => {
        byId(`tab-${tab}`).classList.toggle('active', tab === name);
        byId(`tab-btn-${tab}`).classList.toggle('active', tab === name);
      });
    }

    function buildCompareControls() {
      const groups = compareGroups();
      const scenarioValues = optionValues(groups, group => group.scenario === undefined || group.scenario === null || group.scenario === '' ? 'custom' : String(group.scenario));
      const schedulerValues = optionValues(groups, group => group.server_mode || 'unknown');
      const latencyValues = optionValues(groups, group => group.base_latency_ms === undefined || group.base_latency_ms === null ? 'unknown' : String(group.base_latency_ms));
      const fovValues = optionValues(groups, group => group.fov_mode || 'normal');

      fillSelect('compare-scenario', scenarioValues, value => value === 'all' ? 'All scenarios' : (value === 'custom' ? 'Custom' : humanScenario({ scenario: value })), 'all');
      fillSelect('compare-scheduler', schedulerValues, value => value === 'all' ? 'All schedulers' : value, 'all');
      fillSelect('compare-baselatency', latencyValues, value => value === 'all' ? 'All base latencies' : (value === 'unknown' ? 'Unknown' : `${value} ms`), 'all');
      fillSelect('compare-fov', fovValues, value => value === 'all' ? 'All FoV modes' : value, 'all');

      ['compare-scenario', 'compare-scheduler', 'compare-baselatency', 'compare-fov'].forEach(id => {
        byId(id).addEventListener('change', renderCompareView);
      });
    }

    function buildInspectorControls() {
      const runs = allRuns();
      const select = byId('inspector-run');
      select.innerHTML = runs.map(run => `<option value="${run.id}">${humanRunLabel(run)}</option>`).join('');
      select.addEventListener('change', renderInspectorView);
    }

    function init() {
      buildCompareControls();
      buildInspectorControls();

      byId('tab-btn-compare').addEventListener('click', () => setActiveTab('compare'));
      byId('tab-btn-inspector').addEventListener('click', () => setActiveTab('inspector'));

      const defaultTab = compareGroups().length ? 'compare' : 'inspector';
      setActiveTab(defaultTab);
      renderCompareView();
      renderInspectorView();
    }

    document.addEventListener('DOMContentLoaded', init);
  </script>
</body>
</html>
'''


def generate_dashboard(base_dir, output_path):
    dataset = build_dataset(base_dir)
    html = HTML_TEMPLATE.replace("__DATA_JSON__", json.dumps(dataset))
    with open(output_path, "w", encoding="utf-8") as handle:
        handle.write(html)
    print(f"dashboard written to {output_path}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Generate a static HTML dashboard for ABR validation from client and server logs."
    )
    parser.add_argument(
        "--base-dir",
        default="logs",
        help="Directory containing flat runs or nested run_article_abr_comparison outputs.",
    )
    parser.add_argument(
        "--output",
        default="dashboard.html",
        help="Output HTML file path.",
    )
    arguments = parser.parse_args()
    generate_dashboard(arguments.base_dir, arguments.output)
