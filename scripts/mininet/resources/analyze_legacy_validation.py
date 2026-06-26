#!/usr/bin/env python3
"""Analyze reproducible Legacy-only good/medium/bad validation runs."""

from __future__ import annotations

import csv
import math
import os
import sys
from collections import Counter, defaultdict

CONDITIONS = ("good", "medium", "bad")
TIERS = ("LOW", "MED", "HIGH")
BITRATE_TIER = {3: "LOW", 5: "MED", 10: "HIGH"}
ZONES = ("fov", "near_fov", "background")
MEDIUM_BUFFER_S = 1.0
HIGH_BUFFER_S = 2.0
MEDIUM_THROUGHPUT_MARGIN = 1.05
HIGH_THROUGHPUT_MARGIN = 1.10


def as_bool(value: str) -> bool:
    return str(value).strip().lower() in {"true", "1", "yes"}


def read_env(path: str) -> dict[str, str]:
    result = {}
    with open(path, encoding="utf-8") as handle:
        for raw in handle:
            if "=" in raw and not raw.lstrip().startswith("#"):
                key, value = raw.strip().split("=", 1)
                result[key] = value
    return result


def read_rows(paths: list[str]) -> list[dict[str, str]]:
    rows = []
    for path in paths:
        with open(path, newline="", encoding="utf-8") as handle:
            rows.extend(csv.DictReader(handle))
    return rows


def zone(row: dict[str, str]) -> str:
    if as_bool(row.get("in_fov", "false")):
        return "fov"
    if int(float(row.get("priority", "2") or 2)) == 1:
        return "near_fov"
    return "background"


def longest_streak(decisions: list[dict[str, str]], tier: str) -> int:
    best = current = 0
    for row in decisions:
        if row["tier"] == tier:
            current += 1
            best = max(best, current)
        else:
            current = 0
    return best


def priority_rate(rows: list[dict[str, str]]) -> float:
    by_segment = defaultdict(list)
    for row in rows:
        by_segment[int(row["segment"])].append(row)
    correct = total = 0
    for segment_rows in by_segment.values():
        ordered = sorted(segment_rows, key=lambda r: int(r.get("request_order", "0") or 0))
        ranks = [{"fov": 0, "near_fov": 1, "background": 2}[zone(row)] for row in ordered]
        correct += sum(a <= b for a, b in zip(ranks, ranks[1:]))
        total += max(0, len(ranks) - 1)
    return 100.0 * correct / total if total else 100.0


def collect(root: str) -> dict[str, dict]:
    result = {}
    for condition in CONDITIONS:
        directory = os.path.join(root, condition)
        env = read_env(os.path.join(directory, "experiment.env"))
        if env.get("abr_mode") != "legacy":
            raise ValueError(f"{directory}: not a Legacy-only run")
        names = os.listdir(directory)
        stats = [os.path.join(directory, n) for n in names if n.startswith("statistics-") and n.endswith(".csv") and "summary" not in n]
        debug = [os.path.join(directory, n) for n in names if n.startswith("legacy-decisions") and n.endswith(".csv")]
        if not stats or len(debug) != 1:
            raise ValueError(f"{directory}: expected statistics CSV(s) and exactly one Legacy decision CSV")
        decisions = sorted(read_rows(debug), key=lambda r: int(r["segment"]))
        result[condition] = {"env": env, "rows": read_rows(sorted(stats)), "decisions": decisions}
    return result


def summarize(data: dict[str, dict]) -> tuple[list[dict], list[dict]]:
    summary = []
    per_segment = []
    for condition in CONDITIONS:
        rows = data[condition]["rows"]
        decisions = data[condition]["decisions"]
        tier_counts = Counter(row["tier"] for row in decisions)
        post_warmup = Counter(row["tier"] for row in decisions[5:])
        planned = len(rows)  # skipped rows deliberately remain in every denominator
        completed = sum(as_bool(row.get("ok", "false")) for row in rows)
        on_time = sum(as_bool(row.get("on_time", "false")) for row in rows)
        skipped = sum(as_bool(row.get("skipped", "false")) for row in rows)
        rows_by_segment = defaultdict(list)
        for row in rows:
            rows_by_segment[int(row["segment"])].append(row)
        planned_segments = len(rows_by_segment)
        completed_segments = sum(
            bool(segment_rows) and all(as_bool(row.get("ok", "false")) for row in segment_rows)
            for segment_rows in rows_by_segment.values()
        )
        item = {
            "condition": condition,
            "decisions": len(decisions),
            "planned_tiles": planned,
            "skipped_tiles": skipped,
            "completed_tiles": completed,
            "tile_completion_pct": 100.0 * completed / planned if planned else 0.0,
            "planned_segments": planned_segments,
            "completed_segments": completed_segments,
            "segment_completion_pct": 100.0 * completed_segments / planned_segments if planned_segments else 0.0,
            "tile_on_time_pct": 100.0 * on_time / planned if planned else 0.0,
            "tile_deadline_miss_pct": 100.0 * (planned - on_time) / planned if planned else 0.0,
            "priority_correct_pct": priority_rate(rows),
            "post_warmup_dominant": post_warmup.most_common(1)[0][0] if post_warmup else "",
            "buffer_min_s": min((float(row["buffer_s"]) for row in decisions), default=0.0),
            "buffer_max_s": max((float(row["buffer_s"]) for row in decisions), default=0.0),
        }
        for tier in TIERS:
            item[f"{tier.lower()}_decisions"] = tier_counts[tier]
            item[f"{tier.lower()}_max_streak"] = longest_streak(decisions, tier)
        for spatial in ZONES:
            spatial_rows = [row for row in rows if zone(row) == spatial]
            delivered_rows = [row for row in spatial_rows if as_bool(row.get("ok", "false"))]
            for tier in TIERS:
                count = sum(BITRATE_TIER.get(int(float(row.get("bitrate", "0") or 0))) == tier for row in spatial_rows)
                item[f"{spatial}_{tier.lower()}_pct"] = 100.0 * count / len(spatial_rows) if spatial_rows else 0.0
                delivered = sum(BITRATE_TIER.get(int(float(row.get("bitrate", "0") or 0))) == tier for row in delivered_rows)
                item[f"{spatial}_delivered_{tier.lower()}_pct"] = 100.0 * delivered / len(delivered_rows) if delivered_rows else 0.0
        summary.append(item)

        decision_by_segment = {int(row["segment"]): row for row in decisions}
        for segment in sorted(set(rows_by_segment) | set(decision_by_segment)):
            segment_rows = rows_by_segment[segment]
            decision = decision_by_segment.get(segment, {})
            denominator = len(segment_rows)
            complete_tiles = sum(as_bool(row.get("ok", "false")) for row in segment_rows)
            timely = sum(as_bool(row.get("on_time", "false")) for row in segment_rows)
            segment_completed = denominator > 0 and complete_tiles == denominator
            per_segment.append({
                "condition": condition,
                "segment": segment,
                "tier": decision.get("tier", ""),
                "buffer_s": decision.get("buffer_s", ""),
                "avg_throughput_bps": decision.get("avg_throughput_bps", ""),
                "threshold_med_bps": decision.get("threshold_med_bps", ""),
                "threshold_high_bps": decision.get("threshold_high_bps", ""),
                "planned_tiles": denominator,
                "skipped_tiles": sum(as_bool(row.get("skipped", "false")) for row in segment_rows),
                "completed_tiles": complete_tiles,
                "tile_completion_pct": 100.0 * complete_tiles / denominator if denominator else 0.0,
                "segment_completed": segment_completed,
                "segment_completion_pct": 100.0 if segment_completed else 0.0,
                "tile_on_time_pct": 100.0 * timely / denominator if denominator else 0.0,
                "tile_deadline_miss_pct": 100.0 * (denominator - timely) / denominator if denominator else 0.0,
            })
    return summary, per_segment


def write_csv(path: str, rows: list[dict]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


def plot(root: str, data: dict[str, dict], summary: list[dict], per_segment: list[dict]) -> None:
    import matplotlib.pyplot as plt

    fig, axes = plt.subplots(3, 3, figsize=(16, 11), sharex="col")
    tier_value = {"LOW": 0, "MED": 1, "HIGH": 2}
    for row_index, condition in enumerate(CONDITIONS):
        decisions = data[condition]["decisions"]
        x = [int(row["segment"]) for row in decisions]
        decision_ax, buffer_ax, throughput_ax = axes[row_index]
        decision_ax.step(x, [tier_value[row["tier"]] for row in decisions], where="mid", color="black")
        decision_ax.set_yticks([0, 1, 2], TIERS)
        decision_ax.set_ylabel(condition)
        buffer_ax.plot(x, [float(row["buffer_s"]) for row in decisions], color="tab:blue")
        buffer_ax.set_ylim(-0.05, HIGH_BUFFER_S + 0.05)
        throughput_ax.plot(x, [float(row["avg_throughput_bps"]) for row in decisions], label="EWMA", color="tab:green")
        throughput_ax.plot(x, [float(row["threshold_med_bps"]) for row in decisions], "--", label="MED threshold", color="tab:orange")
        throughput_ax.plot(x, [float(row["threshold_high_bps"]) for row in decisions], "--", label="HIGH threshold", color="tab:red")
    axes[0][0].set_title("LOW / MED / HIGH")
    axes[0][1].set_title("contiguous buffer (s)")
    axes[0][2].set_title("EWMA and real-cost thresholds (B/s)")
    axes[0][2].legend(fontsize=8)
    for ax in axes[-1]:
        ax.set_xlabel("segment")
    fig.tight_layout()
    fig.savefig(os.path.join(root, "legacy_timeline.png"), dpi=160)
    plt.close(fig)

    fig, axes = plt.subplots(2, 3, figsize=(14, 8), sharey=True)
    colors = {"LOW": "tab:blue", "MED": "tab:orange", "HIGH": "tab:red"}
    for column, condition in enumerate(CONDITIONS):
        item = next(row for row in summary if row["condition"] == condition)
        for row_index, delivered in enumerate((False, True)):
            ax = axes[row_index][column]
            bottom = [0.0] * len(ZONES)
            for tier in TIERS:
                infix = "_delivered" if delivered else ""
                values = [item[f"{spatial}{infix}_{tier.lower()}_pct"] for spatial in ZONES]
                ax.bar(ZONES, values, bottom=bottom, label=tier, color=colors[tier])
                bottom = [a + b for a, b in zip(bottom, values)]
            ax.set_title(condition)
            ax.tick_params(axis="x", rotation=20)
    axes[0][0].set_ylabel("requested (%) incl. skipped")
    axes[1][0].set_ylabel("delivered ok (%)")
    axes[0][-1].legend()
    fig.tight_layout()
    fig.savefig(os.path.join(root, "legacy_spatial_quality.png"), dpi=160)
    plt.close(fig)

    fig, axes = plt.subplots(3, 1, figsize=(13, 10), sharex=True, sharey=True)
    for ax, condition in zip(axes, CONDITIONS):
        rows = [row for row in per_segment if row["condition"] == condition]
        x = [row["segment"] for row in rows]
        for key, label in (
            ("tile_completion_pct", "tile completion"),
            ("segment_completion_pct", "segment completion"),
            ("tile_on_time_pct", "tile on-time"),
            ("tile_deadline_miss_pct", "tile deadline miss"),
        ):
            ax.plot(x, [row[key] for row in rows], label=label)
        ax.set_ylabel(f"{condition} (%)")
        ax.legend(ncol=3)
    axes[-1].set_xlabel("segment")
    fig.tight_layout()
    fig.savefig(os.path.join(root, "legacy_delivery_timeline.png"), dpi=160)
    plt.close(fig)


def validate(data: dict[str, dict], summary: list[dict]) -> list[str]:
    errors = []
    by_condition = {row["condition"]: row for row in summary}
    for condition, item in by_condition.items():
        expected = int(data[condition]["env"].get("segment_limit", "60"))
        if item["decisions"] != expected:
            errors.append(f"{condition}: expected {expected} decisions, got {item['decisions']}")
        if not (0 <= item["buffer_min_s"] <= item["buffer_max_s"] <= HIGH_BUFFER_S + 1e-6):
            errors.append(f"{condition}: buffer outside contiguous 0..{HIGH_BUFFER_S:g} s")
        if abs(item["priority_correct_pct"] - 100.0) > 1e-9:
            errors.append(f"{condition}: spatial priority is {item['priority_correct_pct']:.3f}%")
        seen_segments = set()
        for row in data[condition]["decisions"]:
            segment = int(row["segment"])
            if segment in seen_segments:
                errors.append(f"{condition} segment {segment}: duplicate decision")
            seen_segments.add(segment)
            tier = row["tier"]
            throughput = float(row["avg_throughput_bps"])
            buffer_s = float(row["buffer_s"])
            medium_threshold = float(row["threshold_med_bps"])
            high_threshold = float(row["threshold_high_bps"])
            if (
                not math.isfinite(medium_threshold)
                or not math.isfinite(high_threshold)
                or medium_threshold <= 0
                or high_threshold < medium_threshold
            ):
                errors.append(f"{condition} segment {segment}: invalid thresholds")
                continue
            if not math.isfinite(throughput) or throughput <= 0:
                expected_tier = "LOW"
            elif throughput < medium_threshold * MEDIUM_THROUGHPUT_MARGIN:
                expected_tier = "LOW"
            elif throughput >= high_threshold * HIGH_THROUGHPUT_MARGIN:
                expected_tier = "HIGH"
            else:
                expected_tier = "MED"
            if tier != expected_tier:
                errors.append(
                    f"{condition} segment {segment}: tier {tier} violates rule; expected {expected_tier}"
                )
            expected_bitrate = {"LOW": 3, "MED": 5, "HIGH": 10}.get(tier)
            if expected_bitrate is None or int(row["fov_bitrate"]) != expected_bitrate:
                errors.append(f"{condition} segment {segment}: tier/fov bitrate mismatch")
            expected_near = 5 if tier == "HIGH" else 3
            if int(row["near_fov_bitrate"]) != expected_near or int(row["background_bitrate"]) != 3:
                errors.append(f"{condition} segment {segment}: invalid spatial bitrate configuration")
    good, medium, bad = (by_condition[c] for c in CONDITIONS)
    if good["high_max_streak"] < 3 or good["post_warmup_dominant"] != "HIGH":
        errors.append("good: HIGH is not sustained/predominant after warmup")
    if any(medium[f"{tier.lower()}_decisions"] <= 0 for tier in TIERS):
        errors.append("medium: must contain LOW, MED and HIGH decisions")
    if medium["med_max_streak"] < 3:
        errors.append("medium: MED is not sustained for at least 3 decisions")
    if bad["low_decisions"] <= max(bad["med_decisions"], bad["high_decisions"]):
        errors.append("bad: LOW is not predominant")
    if not (good["tile_completion_pct"] >= medium["tile_completion_pct"] >= bad["tile_completion_pct"]):
        errors.append("tile completion does not degrade monotonically")
    if not (good["segment_completion_pct"] >= medium["segment_completion_pct"] >= bad["segment_completion_pct"]):
        errors.append("segment completion does not degrade monotonically")
    if not (good["tile_on_time_pct"] >= medium["tile_on_time_pct"] >= bad["tile_on_time_pct"]):
        errors.append("tile on-time does not degrade monotonically")
    if not (good["tile_deadline_miss_pct"] <= medium["tile_deadline_miss_pct"] <= bad["tile_deadline_miss_pct"]):
        errors.append("tile deadline miss does not increase monotonically")
    return errors


def main() -> int:
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <legacy-validation-root>", file=sys.stderr)
        return 1
    root = os.path.abspath(sys.argv[1])
    data = collect(root)
    summary, per_segment = summarize(data)
    write_csv(os.path.join(root, "legacy_validation_summary.csv"), summary)
    write_csv(os.path.join(root, "legacy_segment_metrics.csv"), per_segment)
    plot(root, data, summary, per_segment)
    errors = validate(data, summary)
    if errors:
        for error in errors:
            print(f"FAIL: {error}", file=sys.stderr)
        return 2
    print("PASS: Legacy validation criteria satisfied")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
