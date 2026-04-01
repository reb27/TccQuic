#!/usr/bin/env python3

import argparse
import csv
import glob
import os
import sys


def load_env(path):
    data = {}
    with open(path, "r", encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or "=" not in line:
                continue
            key, value = line.split("=", 1)
            data[key.strip()] = value.strip()
    return data


def load_single_csv(log_dir, pattern):
    matches = sorted(glob.glob(os.path.join(log_dir, pattern)))
    if not matches:
        raise FileNotFoundError(f"arquivo nao encontrado: {pattern} em {log_dir}")
    if len(matches) > 1:
        raise RuntimeError(f"mais de um arquivo encontrado para {pattern}: {matches}")
    path = matches[0]
    with open(path, "r", encoding="utf-8", newline="") as fh:
        rows = list(csv.DictReader(fh))
    return path, rows


def as_float(row, key):
    return float(row[key])


def as_int(row, key):
    return int(float(row[key]))


def fail(errors, message):
    errors.append(message)


def validate_summary(summary_row, args, errors):
    completion = as_float(summary_row, "segment_completion_rate_percent")
    stale = as_float(summary_row, "stale_bytes_ratio_percent")
    miss_fov = as_float(summary_row, "deadline_miss_rate_fov_percent")
    miss_nonfov = as_float(summary_row, "deadline_miss_rate_nonfov_percent")
    fov_hit = as_float(summary_row, "fov_hit_rate_delivery_percent")
    timely = as_float(summary_row, "timely_bytes_ratio_percent")

    if completion < args.min_completion:
        fail(errors, f"segment_completion_rate_percent={completion:.2f} < minimo {args.min_completion:.2f}")
    if stale > args.max_stale:
        fail(errors, f"stale_bytes_ratio_percent={stale:.2f} > maximo {args.max_stale:.2f}")
    if miss_fov > args.max_deadline_miss_fov:
        fail(errors, f"deadline_miss_rate_fov_percent={miss_fov:.2f} > maximo {args.max_deadline_miss_fov:.2f}")
    if miss_nonfov > args.max_deadline_miss_nonfov:
        fail(errors, f"deadline_miss_rate_nonfov_percent={miss_nonfov:.2f} > maximo {args.max_deadline_miss_nonfov:.2f}")
    if fov_hit < args.min_fov_hit:
        fail(errors, f"fov_hit_rate_delivery_percent={fov_hit:.2f} < minimo {args.min_fov_hit:.2f}")
    if timely < args.min_timely:
        fail(errors, f"timely_bytes_ratio_percent={timely:.2f} < minimo {args.min_timely:.2f}")


def validate_bola(decision_rows, errors):
    expected = {
        "A_all_low": (3, 3),
        "B_fov_med": (5, 3),
        "C_fov_high": (10, 3),
    }
    seen = set()
    for row in decision_rows:
        cfg_id = row["cfg_id"]
        if cfg_id not in expected:
            fail(errors, f"BOLA gerou cfg_id inesperado: {cfg_id}")
            continue
        fov_expected, nonfov_expected = expected[cfg_id]
        fov = as_int(row, "fov_bitrate")
        nonfov = as_int(row, "nonfov_bitrate")
        if (fov, nonfov) != (fov_expected, nonfov_expected):
            fail(errors, f"BOLA inconsistente no segmento {row['segment']}: cfg={cfg_id} mas bitrates=({fov},{nonfov})")
        seen.add(cfg_id)
    if not seen:
        fail(errors, "nenhuma decisao BOLA encontrada")


def validate_legacy(decision_rows, errors):
    allowed_fov = {3, 5, 10}
    for row in decision_rows:
        cfg_id = row["cfg_id"]
        if cfg_id != "default":
            fail(errors, f"legacy gerou cfg_id inesperado no segmento {row['segment']}: {cfg_id}")
        fov = as_int(row, "fov_bitrate")
        nonfov = as_int(row, "nonfov_bitrate")
        if fov not in allowed_fov:
            fail(errors, f"legacy gerou fov_bitrate invalido no segmento {row['segment']}: {fov}")
        if nonfov != 3:
            fail(errors, f"legacy gerou nonfov_bitrate invalido no segmento {row['segment']}: {nonfov}")


def validate_decisions(decision_rows, abr_mode, args, errors):
    if len(decision_rows) < args.min_decisions:
        fail(errors, f"decisoes ABR insuficientes: {len(decision_rows)} < minimo {args.min_decisions}")
    for row in decision_rows:
        total_tiles = as_int(row, "total_tile_count")
        fov_tiles = as_int(row, "fov_tile_count")
        if total_tiles <= 0:
            fail(errors, f"segmento {row['segment']} com total_tile_count invalido: {total_tiles}")
        if fov_tiles < 0 or fov_tiles > total_tiles:
            fail(errors, f"segmento {row['segment']} com fov_tile_count invalido: {fov_tiles}/{total_tiles}")

    mode = abr_mode.strip().lower()
    if mode == "bola":
        validate_bola(decision_rows, errors)
    elif mode in {"legacy", "default", "threshold"}:
        validate_legacy(decision_rows, errors)
    else:
        fail(errors, f"modo ABR nao suportado por este validador: {abr_mode}")


def main():
    parser = argparse.ArgumentParser(description="Valida os CSVs de saida do ABR para bola/legacy.")
    parser.add_argument("log_dir", help="diretorio de log gerado por server_scheduler_test.sh")
    parser.add_argument("--min-completion", type=float, default=95.0)
    parser.add_argument("--max-stale", type=float, default=5.0)
    parser.add_argument("--max-deadline-miss-fov", type=float, default=5.0)
    parser.add_argument("--max-deadline-miss-nonfov", type=float, default=5.0)
    parser.add_argument("--min-fov-hit", type=float, default=95.0)
    parser.add_argument("--min-timely", type=float, default=95.0)
    parser.add_argument("--min-decisions", type=int, default=120)
    args = parser.parse_args()

    env_path = os.path.join(args.log_dir, "experiment.env")
    if not os.path.exists(env_path):
        print(f"ERRO: experiment.env nao encontrado em {args.log_dir}", file=sys.stderr)
        return 2

    env = load_env(env_path)
    abr_mode = env.get("abr_mode", "").strip().lower()
    if not abr_mode:
        print("ERRO: abr_mode ausente em experiment.env", file=sys.stderr)
        return 2

    _, summary_rows = load_single_csv(args.log_dir, "statistics-summary-*.csv")
    _, decision_rows = load_single_csv(args.log_dir, "abr-decisions-*.csv")
    if len(summary_rows) != 1:
        print(f"ERRO: esperado exatamente 1 linha em statistics-summary, obtido {len(summary_rows)}", file=sys.stderr)
        return 2

    summary_row = summary_rows[0]
    errors = []
    validate_summary(summary_row, args, errors)
    validate_decisions(decision_rows, abr_mode, args, errors)

    print(f"log_dir={os.path.abspath(args.log_dir)}")
    print(f"abr_mode={abr_mode}")
    print(
        "summary:"
        f" completion={as_float(summary_row, 'segment_completion_rate_percent'):.2f}%"
        f" stale={as_float(summary_row, 'stale_bytes_ratio_percent'):.2f}%"
        f" miss_fov={as_float(summary_row, 'deadline_miss_rate_fov_percent'):.2f}%"
        f" miss_nonfov={as_float(summary_row, 'deadline_miss_rate_nonfov_percent'):.2f}%"
        f" fov_hit={as_float(summary_row, 'fov_hit_rate_delivery_percent'):.2f}%"
        f" timely={as_float(summary_row, 'timely_bytes_ratio_percent'):.2f}%"
    )
    print(f"abr_decisions={len(decision_rows)} segmentos")

    if errors:
        print("VALIDACAO: FALHOU")
        for message in errors:
            print(f"- {message}")
        return 1

    print("VALIDACAO: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
