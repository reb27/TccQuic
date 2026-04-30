#!/usr/bin/env bash
#
# Focused runs: default is a small sweep (no WFQ/loss matrix) so results are not
# replaced by synthetic plots. For bitrate validation use run_bitrate_smoke.sh (single run).
#
# Usage:
#   ./run_wfq_abr_tile_missing_ratio.sh [--fast|--full|--reuse] <IP>
#
# --fast (default): BG 10, loss 0 only, FIFO, ABR {bola, legacy} = 2 runs
# --full          : WFQ, BG {10,25}, loss {0,5,10,15,20,25}, both ABRs = 24 runs
#

set -euo pipefail

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [--fast|--full|--reuse] <IP>"
    echo "  --fast   Minimal sweep (default): FIFO, 0% loss, both ABRs"
    echo "  --full   Large WFQ × loss matrix (slow)"
    echo "  --reuse  Skip local build on first run"
}

PROFILE="fast"
REUSE_REMOTE=0
IP=
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --fast) PROFILE="fast" ; shift ;;
    --full) PROFILE="full" ; shift ;;
    --reuse) REUSE_REMOTE=1 ; shift ;;
    -*)     showUsage ; exit 1 ;;
    *)      IP="$1" ; shift ;;
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

ABRS=(bola legacy)
SCHEDULER="fifo"
PARALLELISM=24
BASE_LATENCY=450
BW=200
DELAY_MS=24
BACKGROUND_LOADS=(10)
LOSS_RATES=(0)

if [[ "$PROFILE" == "full" ]]; then
    SCHEDULER="wfq"
    BACKGROUND_LOADS=(10 25)
    LOSS_RATES=(0 5 10 15 20 25)
    PARALLELISM=40
fi

FIRST_RUN=1
export SKIP_LOCAL_PLOTS="${SKIP_LOCAL_PLOTS:-1}"

run_one() {
    local bg="$1"
    local loss="$2"
    local abr="$3"

    local log_dir
    log_dir=$(printf "%s/bg%s/loss%s/%s/%s/" \
        "$SUPER_LOG_DIR" "$bg" "$loss" "$SCHEDULER" "$abr")
    mkdir -p "$log_dir"

    cat > "$log_dir/experiment.env" <<EOF
scenario=matrix
matrix_mode=$PROFILE
delay_ms=$DELAY_MS
background_load_pct=$bg
server_mode=$SCHEDULER
abr_mode=$abr
base_latency_ms=$BASE_LATENCY
server_bw_mbps=$BW
client_bw_mbps=$BW
loss_pct=$loss
parallelism=$PARALLELISM
EOF

    local sched_flag=--fifo
    if [[ "$SCHEDULER" == "wfq" ]]; then
        sched_flag=--wfq
    fi

    PARAMS=(-o "$log_dir" $sched_flag --abr "$abr" --sbw "$BW" --cbw "$BW" \
        --loss "$loss" -p "$PARALLELISM" --delay "$DELAY_MS" --load "$bg" \
        --baselatency "$BASE_LATENCY")
    printf "%s\n" "${PARAMS[*]}" > "$log_dir/parameters"

    echo "[INFO] bg=${bg}% sched=$SCHEDULER abr=$abr loss=$loss"

    set +e
    if [[ "$FIRST_RUN" == 1 && "$REUSE_REMOTE" == 0 ]]; then
        ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
    else
        ./server_scheduler_test.sh --no-build "${PARAMS[@]}" "$IP"
    fi
    local run_rc=$?
    set -e
    if [[ "$run_rc" -ne 0 ]]; then
        echo "[WARN] server_scheduler_test.sh saiu com $run_rc (bg=$bg loss=$loss abr=$abr) — seguindo."
    fi
    FIRST_RUN=0
}

for bg in "${BACKGROUND_LOADS[@]}"; do
    for loss in "${LOSS_RATES[@]}"; do
        for abr in "${ABRS[@]}"; do
            run_one "$bg" "$loss" "$abr"
        done
    done
done

python3 resources/plot_tile_missing_ratio.py "$SUPER_LOG_DIR"
python3 resources/plot_bitrate_from_statistics.py "$SUPER_LOG_DIR"
echo "[INFO] Logs: $(cd "$SUPER_LOG_DIR" && pwd)"
