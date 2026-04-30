#!/usr/bin/env python3
"""
Evidência mínima de que existem múltiplas qualidades (representações) no dataset.

1) Tamanhos no disco: para o mesmo (segmento, tile), compara rep 5, 10 e 15
   (alinhado ao servidor: LOW/MEDIUM/HIGH → ficheiros distintos).

2) Opcional: agrega o CSV statistics-* gerado pelo test-client (colunas
   in_fov, bitrate) para mostrar que o cliente pediu bitrates diferentes.

Uso:
  python scripts/summarize_bitrate_evidence.py
  python scripts/summarize_bitrate_evidence.py --segments-dir data/segments
  python scripts/summarize_bitrate_evidence.py --statistics statistics-12345.csv
"""
from __future__ import annotations

import argparse
import csv
import os
import random
import re
from collections import Counter, defaultdict

PATTERN = re.compile(
    r"^video_tiled_(\d+)_dash_track(\d+)_(\d+)\.m4s$",
    re.IGNORECASE,
)

# Mesmo mapeamento que src/server/stream_handler/stream_handler.go
REP_NAMES = {5: "LOW(rep=5)", 10: "MEDIUM(rep=10)", 15: "HIGH(rep=15)"}


def disk_evidence(segments_dir: str, sample_pairs: int, seed: int) -> None:
    by_pair: dict[tuple[int, int], dict[int, int]] = defaultdict(dict)
    for name in os.listdir(segments_dir):
        m = PATTERN.match(name)
        if not m:
            continue
        rep, seg, tile = int(m.group(1)), int(m.group(2)), int(m.group(3))
        if rep not in (5, 10, 15):
            continue
        path = os.path.join(segments_dir, name)
        try:
            sz = os.path.getsize(path)
        except OSError:
            continue
        by_pair[(seg, tile)][rep] = sz

    complete = [(pair, reps) for pair, reps in by_pair.items() if {5, 10, 15} <= set(reps)]
    if not complete:
        print("No (segment, tile) with reps 5, 10 and 15 found in", segments_dir)
        return

    rng = random.Random(seed)
    rng.shuffle(complete)
    picked = complete[: min(sample_pairs, len(complete))]

    print("=== Evidence 1: .m4s sizes on disk (same segment/tile, reps 5/10/15) ===\n")
    for (seg, tile), reps in picked:
        print(f"track {seg}, tile {tile}:")
        for rep in (5, 10, 15):
            label = REP_NAMES.get(rep, f"rep={rep}")
            print(f"  {label:18s}  {reps[rep]:>8d} bytes")
        r5, r10, r15 = reps[5], reps[10], reps[15]
        if r5 < r10 < r15:
            note = "5 < 10 < 15 (typical ladder)"
        elif r5 != r10 or r10 != r15:
            note = "distinct sizes (order may vary)"
        else:
            note = "equal sizes (unexpected)"
        print(f"  => {note}\n")


def statistics_evidence(path: str) -> None:
    with open(path, newline="", encoding="utf-8", errors="replace") as f:
        r = csv.DictReader(f)
        if not r.fieldnames or "bitrate" not in r.fieldnames or "in_fov" not in r.fieldnames:
            print("CSV missing expected columns (bitrate, in_fov):", r.fieldnames)
            return
        c_ok = Counter()
        c_all = Counter()
        for row in r:
            key = (row.get("in_fov", "").strip().lower(), row.get("bitrate", "").strip())
            c_all[key] += 1
            if row.get("ok", "").strip().lower() == "true":
                c_ok[key] += 1

    print("=== Evidence 2: client requests from statistics CSV ===\n")
    print(f"File: {path}\n")
    print("(in_fov, bitrate): total rows / ok=true")
    for key in sorted(c_all, key=lambda k: (k[0], k[1])):
        print(f"  {key[0]!s:5s}  bitrate={key[1]!s:4s}  {c_all[key]:>6d}  /  {c_ok[key]:>6d}")
    bitrates = {k[1] for k in c_all}
    print("\nBitrate values seen:", ", ".join(sorted(bitrates, key=lambda x: int(x) if x.isdigit() else 0)))
    print("(Go model: LOW=3, MEDIUM=5, HIGH=10 -- enum IDs, not kbps.)")


def main() -> None:
    ap = argparse.ArgumentParser(description="Evidência mínima de múltiplas qualidades.")
    ap.add_argument(
        "--segments-dir",
        default="data/segments",
        help="Pasta com video_tiled_*_dash_track*_*.m4s",
    )
    ap.add_argument(
        "--sample-pairs",
        type=int,
        default=4,
        help="Quantos (segmento,tile) completos mostrar",
    )
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument(
        "--statistics",
        default="",
        help="Caminho para statistics-<pid>.csv após um test-client",
    )
    args = ap.parse_args()

    root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    seg_dir = args.segments_dir
    if not os.path.isabs(seg_dir):
        seg_dir = os.path.join(root, seg_dir)

    if os.path.isdir(seg_dir):
        disk_evidence(seg_dir, args.sample_pairs, args.seed)
    else:
        print("Directory not found:", seg_dir)

    if args.statistics:
        stat_path = args.statistics
        if not os.path.isabs(stat_path):
            stat_path = os.path.join(root, stat_path)
        if os.path.isfile(stat_path):
            print()
            statistics_evidence(stat_path)
        else:
            print("Statistics file not found:", stat_path)


if __name__ == "__main__":
    main()
