#!/usr/bin/env python3
"""
Generate synthetic bitrate variants for tiled .m4s segments.

Input files (required):
  video_tiled_10_dash_track{segment}_{tile}.m4s

Output files (created if missing):
  video_tiled_5_dash_track{segment}_{tile}.m4s   (smaller payload)
  video_tiled_15_dash_track{segment}_{tile}.m4s  (larger payload)
"""

from __future__ import annotations

import argparse
import re
from pathlib import Path


BASE_PATTERN = re.compile(r"^video_tiled_10_dash_track(\d+)_(\d+)\.m4s$")


def build_variants(base_dir: Path, low_ratio: float, high_ratio: float) -> tuple[int, int, int]:
    created_low = 0
    created_high = 0
    skipped_empty = 0

    for item in base_dir.iterdir():
        if not item.is_file():
            continue
        match = BASE_PATTERN.match(item.name)
        if not match:
            continue

        segment, tile = match.group(1), match.group(2)
        payload = item.read_bytes()
        if not payload:
            skipped_empty += 1
            continue

        low_size = max(256, int(len(payload) * low_ratio))
        high_extra = max(256, int(len(payload) * high_ratio))
        low_payload = payload[:low_size]
        high_payload = payload + payload[:high_extra]

        low_file = base_dir / f"video_tiled_5_dash_track{segment}_{tile}.m4s"
        high_file = base_dir / f"video_tiled_15_dash_track{segment}_{tile}.m4s"

        if not low_file.exists():
            low_file.write_bytes(low_payload)
            created_low += 1
        if not high_file.exists():
            high_file.write_bytes(high_payload)
            created_high += 1

    return created_low, created_high, skipped_empty


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate synthetic bitrate variants for dataset files.")
    parser.add_argument("--segments-dir", default="data/segments", help="Directory containing .m4s segment files")
    parser.add_argument("--low-ratio", type=float, default=0.6, help="Low bitrate size ratio vs base")
    parser.add_argument("--high-ratio", type=float, default=0.2, help="High bitrate extra ratio vs base")
    args = parser.parse_args()

    segments_dir = Path(args.segments_dir)
    if not segments_dir.exists():
        raise SystemExit(f"Segments directory not found: {segments_dir}")

    base_count = sum(1 for p in segments_dir.iterdir() if p.is_file() and BASE_PATTERN.match(p.name))
    if base_count == 0:
        raise SystemExit("No base files found with pattern video_tiled_10_dash_track{segment}_{tile}.m4s")

    created_low, created_high, skipped_empty = build_variants(
        base_dir=segments_dir,
        low_ratio=args.low_ratio,
        high_ratio=args.high_ratio,
    )
    print(
        f"base={base_count} created_low={created_low} created_high={created_high} "
        f"skipped_empty={skipped_empty}"
    )


if __name__ == "__main__":
    main()
