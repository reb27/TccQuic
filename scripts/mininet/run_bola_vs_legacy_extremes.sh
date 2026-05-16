#!/usr/bin/env bash
#
# Depends on: ./server_scheduler_test.sh, resources/server_scheduler_test.py, resources/utils.py,
#             resources/analyze_bola_legacy_extremes.py (after runs).
#
# Quick Mininet comparison: BOLA vs Legacy under two extreme network conditions.
#
# Uses existing dataset as-is (including reps 5/10/15 when already present).
#
# Runs:
#   2 conditions (good/bad) x 2 ABRs (bola, legacy) = 4 runs
#
# Outputs in one super-log folder:
#   - abr_extremes_summary.csv
#   - abr_extremes_quality.png
#   - abr_extremes_dashboard.png
#   - abr_extremes_spatial_mix.png (ok=true: barras agrupadas FoV/perto/fundo, contagens + eixo log)
#   - abr_extremes_delivered_counts.png
#
# Usage:
#   ./run_bola_vs_legacy_extremes.sh [--reuse] <IP>
#
# --reuse   Skip first build/upload and reuse remote binary (--no-build in all runs).
#

set -euo pipefail

PROGRAM_NAME=$0
show_usage() {
    echo "Usage: $PROGRAM_NAME [--reuse] <IP>"
}

REUSE_REMOTE=0
IP=
while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --reuse) REUSE_REMOTE=1 ; shift ;;
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

# keep run shorter; we only need contrast between two extremes.
# format: id|label|client_bw|loss|delay_ms|background_load|base_latency|parallelism
CONDITIONS=(
    "good|Boa (link limpo)|200|0|8|0|220|18"
    "bad|Ruim (perda + carga)|40|20|45|25|700|18"
)
ABRS=(bola legacy)
SCHEDULER="wfq"
SERVER_BW=200

run_idx=0
for condition_spec in "${CONDITIONS[@]}"; do
    IFS='|' read -r CID CLABEL CBW LOSS DELAY BG BASELAT PAR <<< "$condition_spec"
    for ABR in "${ABRS[@]}"; do
        log_dir="${SUPER_LOG_DIR%/}/${CID}/${ABR}"
        mkdir -p "$log_dir"

        cat > "$log_dir/experiment.env" <<EOF
scenario=abr_extremes
scenario_family=bola_legacy_extremes
condition_id=$CID
condition_label=$CLABEL
plot_label=${CLABEL} - ${ABR}
delay_ms=$DELAY
background_load_pct=$BG
server_mode=$SCHEDULER
abr_mode=$ABR
base_latency_ms=$BASELAT
server_bw_mbps=$SERVER_BW
client_bw_mbps=$CBW
loss_pct=$LOSS
parallelism=$PAR
fov_mode=normal
EOF

        PARAMS=(-o "$log_dir" --"$SCHEDULER" --abr "$ABR" --sbw "$SERVER_BW" --cbw "$CBW" \
            --loss "$LOSS" -p "$PAR" --delay "$DELAY" --load "$BG" \
            --baselatency "$BASELAT" --fov normal)
        printf "%s\n" "${PARAMS[*]}" > "$log_dir/parameters"

        echo "[INFO] condition=$CID abr=$ABR cbw=$CBW loss=$LOSS delay=$DELAY bg=$BG"
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
            echo "[WARN] server_scheduler_test.sh saiu com código $rc (condition=$CID abr=$ABR)."
            if [[ "$did_build" -eq 1 ]]; then
                echo "[ERROR] 1a corrida (build/upload) falhou — interrompendo. Ver README.md (requisitos)."
                exit "$rc"
            fi
        fi
        run_idx=$((run_idx + 1))
    done
done

python3 resources/analyze_bola_legacy_extremes.py "$SUPER_LOG_DIR"
echo "[INFO] Super-log: $(cd "$SUPER_LOG_DIR" && pwd)"
