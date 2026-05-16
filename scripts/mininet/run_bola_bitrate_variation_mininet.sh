#!/usr/bin/env bash
#
# Mininet: várias corridas com ABR=BOLA e banda do cliente distinta, para ver
# a mistura de bitrate pedida (dataset em data/segments com reps 5/10/15, etc.).
#
# Por defeito NÃO altera data/segments — usa o que já tens.
#
# Ao final gera um gráfico agrupado:
#   <SUPER_LOG>/bola_bitrate_por_cenario.png
#
# Uso:
#   ./run_bola_bitrate_variation_mininet.sh [--reuse] [--generate-variants] <IP>
#
# --reuse              Só na 1ª corrida: mesmo efeito que server_scheduler_test --no-build
#                      (demais corridas sempre --no-build).
# --generate-variants  Corre generate_bitrate_variants.py (só útil se faltar rep 5/15
#                      sintéticas a partir da base rep 10).
#

set -euo pipefail

PROGRAM_NAME=$0
show_usage() {
    echo "Usage: $PROGRAM_NAME [--reuse] [--generate-variants] <IP>"
}

REUSE_REMOTE=0
GENERATE_VARIANTS=0
IP=
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --reuse) REUSE_REMOTE=1 ; shift ;;
    --generate-variants) GENERATE_VARIANTS=1 ; shift ;;
    -*) show_usage ; exit 1 ;;
    *) IP="$1" ; shift ;;
    esac
done

if [[ -z "$IP" ]]; then
    show_usage
    exit 1
fi

cd -- "$( dirname -- "${BASH_SOURCE[0]}" )"

LOG_NUMBER=1
while true; do
    SUPER_LOG_DIR=$(printf "../../logs/%s-%03d/" \
        "$(basename "${PROGRAM_NAME%.*}")" "$LOG_NUMBER")
    if [[ ! -e "$SUPER_LOG_DIR" ]]; then
        break
    fi
    LOG_NUMBER=$((LOG_NUMBER + 1))
done
mkdir -p "$SUPER_LOG_DIR"

if [[ "$GENERATE_VARIANTS" == "1" ]]; then
    echo "[INFO] --generate-variants: a criar/atualizar reps 5 e 15 sintéticas em ../../data/segments..."
    python3 resources/generate_bitrate_variants.py --segments-dir ../../data/segments
else
    echo "[INFO] A usar data/segments tal como está (sem generate_bitrate_variants)."
fi

# nome_curto|client_bw_mbps — ordem: baixa → alta (efeito típico no BOLA)
SCENARIOS=(
    "cbw030|30"
    "cbw080|80"
    "cbw150|150"
    "cbw220|220"
)

SCHEDULER="fifo"
ABR="bola"
SERVER_BW=200
PARALLELISM=24
BASE_LATENCY=450
DELAY_MS=24
BG=10
LOSS=0

export SKIP_LOCAL_PLOTS="${SKIP_LOCAL_PLOTS:-1}"

run_idx=0
for spec in "${SCENARIOS[@]}"; do
    IFS='|' read -r TAG CBW <<< "$spec"
    log_dir="${SUPER_LOG_DIR%/}/${TAG}"
    mkdir -p "$log_dir"

    cat > "$log_dir/experiment.env" <<EOF
scenario=${TAG}
plot_label=Cliente ${CBW} Mbps
scenario_family=bola_bitrate_variation_mininet
delay_ms=${DELAY_MS}
background_load_pct=${BG}
server_mode=${SCHEDULER}
abr_mode=${ABR}
base_latency_ms=${BASE_LATENCY}
server_bw_mbps=${SERVER_BW}
client_bw_mbps=${CBW}
loss_pct=${LOSS}
parallelism=${PARALLELISM}
fov_mode=normal
EOF

    PARAMS=(-o "$log_dir" --"${SCHEDULER}" --abr "$ABR" --sbw "$SERVER_BW" --cbw "$CBW" \
        --loss "$LOSS" -p "$PARALLELISM" --delay "$DELAY_MS" --load "$BG" \
        --baselatency "$BASE_LATENCY" --fov normal)

    echo "[INFO] Cenário ${TAG} client_bw=${CBW} Mbps sched=${SCHEDULER} abr=${ABR}"

    set +e
    did_build=0
    if [[ "$run_idx" -eq 0 && "$REUSE_REMOTE" == "0" ]]; then
        did_build=1
        ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
    else
        ./server_scheduler_test.sh --no-build "${PARAMS[@]}" "$IP"
    fi
    rc=$?
    set -e
    if [[ "$rc" -ne 0 ]]; then
        echo "[WARN] server_scheduler_test.sh saiu com código $rc para ${TAG}."
        if [[ "$did_build" -eq 1 ]]; then
            echo "[ERROR] 1ª corrida (build/upload) falhou — interrompendo. Ver README.md (requisitos)."
            exit "$rc"
        fi
    fi
    run_idx=$((run_idx + 1))
done

python3 resources/plot_bola_bitrate_scenarios.py "$SUPER_LOG_DIR"
echo "[INFO] Super-log: $(cd "$SUPER_LOG_DIR" && pwd)"
