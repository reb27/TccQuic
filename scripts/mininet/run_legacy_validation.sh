#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 1 ]]; then
    echo "Usage: $0 <mininet-ip>"
    exit 1
fi

IP=$1
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

run_number=1
while true; do
    ROOT=$(printf "../../logs/legacy-validation-%03d" "$run_number")
    if [[ ! -e "$ROOT" ]]; then
        break
    fi
    run_number=$((run_number + 1))
done
mkdir -p "$ROOT"

# id|server Mbps|client Mbps|loss %|delay ms|load %|base latency ms|parallelism
CONDITIONS=(
    "good|200|200|0|8|0|220|18"
    "medium|200|145|1.8|14|5|335|18"
    "bad|200|130|3|15|5|350|18"
)

reuse=0
for spec in "${CONDITIONS[@]}"; do
    IFS='|' read -r id sbw cbw loss delay load base parallelism <<< "$spec"
    out="$ROOT/$id"
    mkdir -p "$out"
    cat > "$out/experiment.env" <<EOF
scenario_family=legacy_validation
condition_id=$id
abr_mode=legacy
server_mode=wfq
server_bw_mbps=$sbw
client_bw_mbps=$cbw
loss_pct=$loss
delay_ms=$delay
background_load_pct=$load
base_latency_ms=$base
parallelism=$parallelism
fov_mode=normal
segment_limit=60
legacy_debug_path=legacy-decisions.csv
delivery_evidence=client_tile_and_segment_metrics
server_summary_policy=exclude_if_header_only
EOF

    cmd=(./server_scheduler_test.sh --wfq --abr legacy --sbw "$sbw" --cbw "$cbw" \
        --loss "$loss" --delay "$delay" --load "$load" --baselatency "$base" \
        -p "$parallelism" --fov normal -o "$out")
    if [[ "$reuse" -eq 1 ]]; then
        cmd+=(--no-build)
    fi
    cmd+=("$IP")
    printf '%q ' env LEGACY_DEBUG_PATH=legacy-decisions.csv TEST_CLIENT_SEGMENT_LIMIT=60 "${cmd[@]}" > "$out/command.txt"
    printf '\n' >> "$out/command.txt"
    LEGACY_DEBUG_PATH=legacy-decisions.csv TEST_CLIENT_SEGMENT_LIMIT=60 "${cmd[@]}"
    reuse=1
done

python3 resources/analyze_legacy_validation.py "$ROOT"
echo "Legacy validation artifacts: $(cd "$ROOT" && pwd)"
