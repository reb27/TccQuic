#!/usr/bin/env bash

set -euo pipefail

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME <IP>"
}

if [[ "$#" != 1 ]]; then
    showUsage
    exit 1
fi

IP="$1"

cd -- "$( dirname -- "${BASH_SOURCE[0]}" )"

LOG_NUMBER=1
while true; do
    SUPER_LOG_DIR=$(printf "../../logs/%s-%03d/" \
        "$(basename "${PROGRAM_NAME%.*}")" "$LOG_NUMBER")
    if [[ ! -e "$SUPER_LOG_DIR" ]]; then
        break
    fi
    LOG_NUMBER=$((LOG_NUMBER+1))
done
mkdir -p "$SUPER_LOG_DIR"

run_case() {
    local name="$1"
    local abr="$2"
    local sbw="$3"
    local cbw="$4"
    local loss="$5"
    local delay="$6"
    local load="$7"
    local baselatency="$8"
    local parallelism="$9"

    local log_dir="$SUPER_LOG_DIR/$name"
    echo "[INFO] case=$name abr=$abr sbw=$sbw cbw=$cbw loss=$loss delay=$delay load=$load baselatency=$baselatency"

    ./server_scheduler_test.sh \
        --wfq \
        --abr "$abr" \
        --sbw "$sbw" \
        --cbw "$cbw" \
        --loss "$loss" \
        --delay "$delay" \
        --load "$load" \
        --baselatency "$baselatency" \
        -p "$parallelism" \
        -o "$log_dir" \
        "$IP"

    python3 resources/validate_abr_metrics.py "$log_dir"
}

# Cenarios de validacao recomendados:
# 1) sanity: rede folgada para esperar metricas finais proximas de 100/0
# 2) constrained: rede mais apertada para inspecionar as decisoes do ABR no CSV abr-decisions
run_case "bola_sanity" "bola" 100 100 0 5 0 120 120
run_case "legacy_sanity" "legacy" 100 100 0 5 0 120 120
run_case "bola_constrained" "bola" 60 50 4 20 30 800 120
run_case "legacy_constrained" "legacy" 60 50 4 20 30 800 120

echo "[INFO] Logs: $(cd "$SUPER_LOG_DIR" && pwd)"
