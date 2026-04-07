#!/usr/bin/env bash
#
# ABR × scheduler × loss × background traffic matrix for Mininet experiments.
#
# Usage: ./run_article_abr_comparison.sh [--matrix|--fast|--pilot|--legacy|--full] <IP>
#
#   --matrix (default)  2 BG (10%,25%) × 6 losses × 3 policies × 2 ABR = 72 runs
#                       Policies: fifo, sp, wfq
#   --fast              Mesma malha que --matrix, porém 3 pontos de perda (0,10,25%) e
#                       paralelismo menor → ~36 runs, cada run tende a ser mais curto
#   --pilot             Smoke test: 1 BG, 2 losses, fifo+wfq, 2 ABR (~8 runs)
#   --legacy            Old triple-scenario layout (SP/WFQ only, old plot path)
#   --full              Legacy profile + many base latencies (heavy)
#
# Matriz (--matrix / --fast / --pilot): exporta SKIP_LOCAL_PLOTS=1 para cada
# server_scheduler_test.sh (sem PNGs por pasta). Gráfico agregado: plot_tile_missing_ratio.py.
# Para manter plots por corrida: SKIP_LOCAL_PLOTS=0 ./run_article_abr_comparison.sh ...
#

set -euo pipefail

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [--matrix|--fast|--pilot|--legacy|--full] <IP>"
    echo "  --matrix   Full matrix: FIFO, SP, WFQ × BOLA/Legacy × loss × BG 10%/25%"
    echo "  --fast     Matrix enxuta (3 perdas, menos paralelismo) para rehearsal rápida"
    echo "  --pilot    Quick validation (fewer runs)"
    echo "  --legacy   Previous scenario-based sweep (sp/wfq only)"
    echo "  --full     Legacy + many base_latency values"
}

PROFILE="matrix"
MATRIX_MODE="full"
IP=
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --matrix)  PROFILE="matrix"  ; shift ;;
    --fast)    PROFILE="matrix"  ; MATRIX_MODE="fast" ; shift ;;
    --pilot)   PROFILE="pilot"   ; shift ;;
    --legacy)  PROFILE="legacy"  ; shift ;;
    --full)    PROFILE="full"    ; shift ;;
    -*)        showUsage ; exit 1 ;;
    *)         IP="$1"           ; shift ;;
    esac
done

if [[ -z "$IP" ]]; then
    showUsage
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
    LOG_NUMBER=$((LOG_NUMBER+1))
done
mkdir -p "$SUPER_LOG_DIR"

# --- matrix profile (paper) ---
# Tuned for interpretable curves: lower parallelism + higher base latency + ample link capacity.
BACKGROUND_LOADS=(10 25)
SCHEDULERS=(fifo sp wfq)
ABRS=(bola legacy)
LOSS_RATES=(0 5 10 15 20 25)
PARALLELISM=40
BASE_LATENCY=450
BW=200
DELAY_MS=24

case "$PROFILE" in
matrix)
    if [[ "$MATRIX_MODE" == "fast" ]]; then
        # Menos pontos no eixo X e menos pedidos concorrentes por run.
        LOSS_RATES=(0 10 25)
        PARALLELISM=24
    fi
    ;;
pilot)
    BACKGROUND_LOADS=(10)
    SCHEDULERS=(fifo wfq)
    LOSS_RATES=(0 10)
    PARALLELISM=40
    BASE_LATENCY=450
    BW=200
    ;;
legacy)
    BASE_LATENCIES=(500)
    LEGACY_SCHEDULERS=(sp wfq)
    LEGACY_ABRS=(bola legacy)
    LOSS_RATES=(0 2 5 10 15 20 25)
    SCENARIOS=(
        "1 10 24"
        "3 10 16"
        "6 30 10"
    )
    ;;
full)
    BASE_LATENCIES=(10 50 100 200 300 400 500 1000)
    LEGACY_SCHEDULERS=(sp wfq)
    LEGACY_ABRS=(bola legacy)
    LOSS_RATES=(0 1 2 3 5 8 10 12 15 18 20 25)
    SCENARIOS=(
        "1 10 24"
        "3 10 16"
        "6 30 10"
    )
    ;;
*)
    echo "Unknown profile: $PROFILE"
    exit 1
    ;;
esac

FIRST_RUN=1

scheduler_flag() {
    # printf: evita edge cases do echo com argumentos que começam com --
    case "$1" in
        fifo)   printf '%s\n' --fifo ;;
        sp)     printf '%s\n' --sp ;;
        wfq)    printf '%s\n' --wfq ;;
        *)      echo "unknown scheduler: $1" >&2; exit 1 ;;
    esac
}

launchMatrix() {
    # Evita matplotlib por corrida; o gráfico principal é plot_tile_missing_ratio.py no fim.
    export SKIP_LOCAL_PLOTS="${SKIP_LOCAL_PLOTS:-1}"
    local bg loss sched abr flag
    for bg in "${BACKGROUND_LOADS[@]}"; do
        for loss in "${LOSS_RATES[@]}"; do
            for sched in "${SCHEDULERS[@]}"; do
                for abr in "${ABRS[@]}"; do
                    LOG_DIR=$(printf "%s/bg%s/loss%s/%s/%s/" \
                        "$SUPER_LOG_DIR" "$bg" "$loss" "$sched" "$abr")
                    mkdir -p "$LOG_DIR"
                    cat > "$LOG_DIR/experiment.env" <<EOF
scenario=matrix
matrix_mode=$MATRIX_MODE
delay_ms=$DELAY_MS
background_load_pct=$bg
server_mode=$sched
abr_mode=$abr
base_latency_ms=$BASE_LATENCY
server_bw_mbps=$BW
client_bw_mbps=$BW
loss_pct=$loss
parallelism=$PARALLELISM
EOF
                    flag=$(scheduler_flag "$sched")
                    # shellcheck disable=SC2206
                    PARAMS=(-o "$LOG_DIR" $flag --abr "$abr" --sbw "$BW" --cbw "$BW" \
                        --loss "$loss" -p "$PARALLELISM" --delay "$DELAY_MS" --load "$bg" \
                        --baselatency "$BASE_LATENCY")
                    printf "%s\n" "${PARAMS[*]}" > "$LOG_DIR/parameters"
                    echo "[INFO] bg=${bg}% sched=$sched abr=$abr loss=$loss"

                    set +e
                    if [[ "$FIRST_RUN" == 1 ]]; then
                        ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
                    else
                        ./server_scheduler_test.sh --no-build "${PARAMS[@]}" "$IP"
                    fi
                    run_rc=$?
                    set -e
                    if [[ "$run_rc" -ne 0 ]]; then
                        echo "[WARN] server_scheduler_test.sh saiu com $run_rc (bg=$bg loss=$loss sched=$sched abr=$abr) — seguindo para a próxima combinação."
                    fi
                    if [[ "$FIRST_RUN" == 1 ]]; then
                        FIRST_RUN=0
                    fi
                done
            done
        done
    done
}

launchLegacy() {
    local scenario="$1"
    local load="$2"
    local delay="$3"
    local parallelism=120
    local bw=100

    for base_latency in "${BASE_LATENCIES[@]}"; do
      for loss in "${LOSS_RATES[@]}"; do
        for scheduler in "${LEGACY_SCHEDULERS[@]}"; do
            for abr in "${LEGACY_ABRS[@]}"; do
                LOG_DIR=$(printf "%s/scenario%s-baselatency%s-loss%s/%s/%s/" \
                    "$SUPER_LOG_DIR" "$scenario" "$base_latency" "$loss" "$scheduler" "$abr")
                mkdir -p "$LOG_DIR"
                cat > "$LOG_DIR/experiment.env" <<EOF
scenario=$scenario
delay_ms=$delay
background_load_pct=$load
server_mode=$scheduler
abr_mode=$abr
base_latency_ms=$base_latency
server_bw_mbps=$bw
client_bw_mbps=$bw
loss_pct=$loss
parallelism=$parallelism
EOF

                PARAMS=(-o "$LOG_DIR" "--$scheduler" --abr "$abr" --sbw "$bw" --cbw "$bw" \
                    --loss "$loss" -p "$parallelism" --delay "$delay" --load "$load" \
                    --baselatency "$base_latency")
                printf "%s\n" "${PARAMS[*]}" > "$LOG_DIR/parameters"
                echo "[INFO] scenario=$scenario scheduler=$scheduler abr=$abr loss=$loss base_latency=$base_latency"

                if [[ "$FIRST_RUN" == 1 ]]; then
                    ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
                    FIRST_RUN=0
                else
                    ./server_scheduler_test.sh --no-build "${PARAMS[@]}" "$IP"
                fi
            done
        done
      done
    done
}

case "$PROFILE" in
matrix|pilot)
    echo "[INFO] profile=$PROFILE matrix_mode=$MATRIX_MODE bg=${#BACKGROUND_LOADS[@]} schedulers=${#SCHEDULERS[@]} abrs=${#ABRS[@]} losses=${#LOSS_RATES[@]} (total runs $((${#BACKGROUND_LOADS[@]}*${#LOSS_RATES[@]}*${#SCHEDULERS[@]}*${#ABRS[@]})))"
    launchMatrix
    python3 resources/plot_tile_missing_ratio.py "$SUPER_LOG_DIR"
    ;;
legacy|full)
    echo "[INFO] profile=$PROFILE scenarios=${#SCENARIOS[@]}"
    for scenario_def in "${SCENARIOS[@]}"; do
        # shellcheck disable=SC2086
        launchLegacy $scenario_def
    done
    python3 resources/plot_tile_missing_ratio.py \
        "$SUPER_LOG_DIR" "$SUPER_LOG_DIR/tile_missing_ratio.png"
    ;;
esac

echo "[INFO] Logs: $(cd "$SUPER_LOG_DIR" && pwd)"
