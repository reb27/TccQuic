#!/usr/bin/env bash
# Fast checks for Mininet helper scripts (no network / Mininet).
# Usage: from repo root or from this directory:
#   bash scripts/mininet/run_script_unit_tests.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

export MPLBACKEND="${MPLBACKEND:-Agg}"

echo "[unittest] plot_tile_missing_ratio (scripts/mininet/resources)"
python3 -m unittest discover -s "${SCRIPT_DIR}/resources" -p "test_*.py" -v

echo "[bash -n] shell scripts"
for f in run_article_abr_comparison.sh server_scheduler_test.sh; do
  bash -n "${SCRIPT_DIR}/${f}"
  echo "  OK ${f}"
done

echo "[done] all script unit checks passed"
