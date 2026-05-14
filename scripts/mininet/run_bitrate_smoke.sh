#!/usr/bin/env bash
#
# Minimal run: one scheduler (default WFQ), one ABR (BOLA), no loss sweep — validates
# multi-representation paths end-to-end. Produces bitrate_mix.png from real CSVs.
# When request_order exists, also produces fov_request_order.png.
#
# Usage:
#   ./run_bitrate_smoke.sh [--reuse] [--scheduler fifo|sp|wfq] <IP>
#
# --reuse  Skip local/remote build on first invocation (same as server_scheduler_test --no-build).
#

set -euo pipefail

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [--reuse] [--scheduler fifo|sp|wfq] <IP>"
}

scheduler_flag() {
    case "$1" in
        fifo) printf '%s\n' --fifo ;;
        sp)   printf '%s\n' --sp ;;
        wfq)  printf '%s\n' --wfq ;;
        *)    echo "unknown scheduler: $1" >&2; exit 1 ;;
    esac
}

REUSE_REMOTE=0
SCHEDULER="wfq"
IP=
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --reuse)     REUSE_REMOTE=1 ; shift ;;
    --scheduler)
        if [[ "$#" -lt 2 ]]; then showUsage; exit 1; fi
        SCHEDULER="$2"
        shift 2
        ;;
    -*)          showUsage ; exit 1 ;;
    *)           IP="$1" ; shift ;;
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

ABR="bola"
PARALLELISM=60
BASE_LATENCY=450
BW=60
DELAY_MS=24
BG=10
LOSS=10
SCHEDULER_FLAG=$(scheduler_flag "$SCHEDULER")

export SKIP_LOCAL_PLOTS="${SKIP_LOCAL_PLOTS:-1}"

log_dir=$(printf "%s/bg%s/loss%s/%s/%s/" \
    "$SUPER_LOG_DIR" "$BG" "$LOSS" "$SCHEDULER" "$ABR")
mkdir -p "$log_dir"

cat > "$log_dir/experiment.env" <<EOF
scenario=bitrate_smoke
delay_ms=$DELAY_MS
background_load_pct=$BG
server_mode=$SCHEDULER
abr_mode=$ABR
base_latency_ms=$BASE_LATENCY
server_bw_mbps=$BW
client_bw_mbps=$BW
loss_pct=$LOSS
parallelism=$PARALLELISM
EOF

PARAMS=(-o "$log_dir" "$SCHEDULER_FLAG" --abr "$ABR" --sbw "$BW" --cbw "$BW" \
    --loss "$LOSS" -p "$PARALLELISM" --delay "$DELAY_MS" --load "$BG" \
    --baselatency "$BASE_LATENCY")
printf "%s\n" "${PARAMS[*]}" > "$log_dir/parameters"

echo "[INFO] bitrate_smoke sched=$SCHEDULER abr=$ABR loss=$LOSS bg=$BG"

set +e
if [[ "$REUSE_REMOTE" == 0 ]]; then
    ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
else
    ./server_scheduler_test.sh --no-build "${PARAMS[@]}" "$IP"
fi
run_rc=$?
set -e
if [[ "$run_rc" -ne 0 ]]; then
    echo "[WARN] server_scheduler_test.sh exited $run_rc — check SSH and VM logs."
fi

python3 resources/plot_bitrate_from_statistics.py "$SUPER_LOG_DIR"
echo "[INFO] Logs: $(cd "$SUPER_LOG_DIR" && pwd)"
