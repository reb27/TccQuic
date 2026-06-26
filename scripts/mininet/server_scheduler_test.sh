#!/usr/bin/env bash

set -euo pipefail

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [OPTIONS] <IP>"
    echo "OPTIONS:"
    echo "--fifo, --sp, --wfq  Select server mode (default: fifo)"
    echo "--abr MODE              Select client ABR mode (bola|legacy)"
    echo "--bola-qmax N           Select BOLA Qmax in segments"
    echo "--sbw N                 Select server bandwidth in Mbps"
    echo "--cbw N                 Select client bandwidth in Mbps"
    echo "--baselatency N         Select client base latency"
    echo "--loss N                Select loss in %"
    echo "-p N                    Select paralellism"
    echo "--delay N               Select delay"
    echo "--load N                Select load %"
    echo "--fov NAME              Select FoV trace: narrow, normal, wide"
    echo "-o DIR                  Select output directory"
    echo "--no-build              Skip compile & upload (reuse existing remote binary)"
}

SERVER_MODE="wfq"
ABR_MODE="bola"
SERVER_BW="60"
CLIENT_BW="60"
LOSS="10"
PARALELLISM="120"
DELAY="2"
LOAD="10"
BASE_LATENCY="700"
FOV_MODE="normal"
NUM_CLIENTS="1"
FOV_MIX="balanced"
WFQ_BETA="1.0"
NO_BUILD=0
IP=
LOG_DIR=
BOLA_DEBUG_PATH="${BOLA_DEBUG_PATH:-}"
BOLA_QMAX_SEGMENTS="${BOLA_QMAX_SEGMENTS:-5}"
LEGACY_DEBUG_PATH="${LEGACY_DEBUG_PATH:-}"
TEST_CLIENT_SEGMENT_LIMIT="${TEST_CLIENT_SEGMENT_LIMIT:-0}"

while [[ "$#" > 0 ]]; do
    case "$1" in
    --fifo)   SERVER_MODE="fifo"            ; shift   ;;
    --sp)     SERVER_MODE="sp"              ; shift   ;;
    --wfq)    SERVER_MODE="wfq"             ; shift   ;;
    --abr)  ABR_MODE="$2"                   ; shift 2 ;;
    --bola-qmax) BOLA_QMAX_SEGMENTS="$2"    ; shift 2 ;;
    --sbw)  SERVER_BW="$2"                  ; shift 2 ;;
    --cbw)  CLIENT_BW="$2"                  ; shift 2 ;;
    --baselatency)  BASE_LATENCY="$2"       ; shift 2 ;;
    --loss) LOSS="$2"                       ; shift 2 ;;
    -p)     PARALELLISM="$2"                ; shift 2 ;;
    --delay) DELAY="$2"                     ; shift 2 ;;
    --load) LOAD="$2"                       ; shift 2 ;;
    --fov)   FOV_MODE="$2"                  ; shift 2 ;;
    --clients) NUM_CLIENTS="$2"             ; shift 2 ;;
    --fov-mix) FOV_MIX="$2"                ; shift 2 ;;
    --beta) WFQ_BETA="$2"                   ; shift 2 ;;
    -o)     LOG_DIR="$2"                    ; shift 2 ;;
    --no-build) NO_BUILD=1                  ; shift   ;;
    -*)     showUsage ; exit 1              ; shift   ;;
    *)      IP="$1"                         ; shift   ;;
    esac
done

if [[ -z $IP ]]; then
    showUsage
    exit 1
fi

############################################################################

cd -- "$( dirname -- "${BASH_SOURCE[0]}" )"

PURPLE='\033[0;35m'
NC='\033[0m'

if [[ -z "$LOG_DIR" ]]; then
    LOG_NUMBER=1
    while true; do
        LOG_DIR=$(printf "../../logs/%s/%s-sbw%s-cbw%s-%03d/" \
            $(basename "${PROGRAM_NAME%.*}") "$SERVER_MODE-$ABR_MODE" $SERVER_BW \
            $CLIENT_BW $LOG_NUMBER)
        if [[ ! -e "$LOG_DIR" ]]; then
            break
        fi
        LOG_NUMBER=$((LOG_NUMBER+1))
    done
fi

# withSSH COMMAND ...
withSSH() {
    ssh -t -Y -q -oBatchMode=yes -oConnectTimeout=5 "mininet@$IP" "$@"
    EXIT_CODE=$?
    if [[ $EXIT_CODE == 255 ]]; then
        echo
        echo "SSH connection failed!"
        echo
        echo "Verify if it is possible to login via ssh without a password,"\
             "using the following command:"
        echo -e "${PURPLE}\$ ssh mininet@$IP${NC}"
        echo "If a password is required, upload your public key using"\
             "the script upload_ssh_key.sh in order to be able to login"\
             "without a password."
        echo
        exit $EXIT_CODE
    fi
    return $EXIT_CODE
}

# upload SOURCE DESTINATION
upload() {
    scp -r "$1" "mininet@$IP:$2"
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "scp upload failed!"
        echo
        exit $EXIT_CODE
    fi
}

# download SOURCE DESTINATION
download() {
    # scp não expande curingas remotamente, então usamos tar para baixar múltiplos arquivos
    if ! withSSH "cd $REMOTE_DIR && ls ./*.csv >/dev/null 2>&1"; then
        echo -e "${PURPLE}[warn] Nenhum .csv em $REMOTE_DIR — nada a baixar (teste pode ter falhado cedo).${NC}"
        return 0
    fi
    if ! withSSH "cd $REMOTE_DIR && tar czf results.tar.gz ./*.csv"; then
        echo -e "${PURPLE}[warn] Falha ao criar results.tar.gz no remoto.${NC}"
        return 0
    fi
    scp "mininet@$IP:$REMOTE_DIR/results.tar.gz" "$2"
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "scp download failed!"
        echo
        return "$EXIT_CODE"
    fi
    (cd "$2" && tar xzf results.tar.gz && rm results.tar.gz)
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "tar extract failed!"
        echo
        return "$EXIT_CODE"
    fi
    # The harness terminates the long-lived server after the client exits. If
    # that prevents the graceful shutdown hook from writing a data row, do not
    # leave a header-only CSV that could be mistaken for validation evidence.
    local server_summary="$2/server_summary.csv"
    if [[ -f "$server_summary" ]] && [[ "$(wc -l < "$server_summary")" -le 1 ]]; then
        rm -f "$server_summary"
        printf '%s\n' \
            'excluded: server process termination produced no summary data row; client tile/segment metrics are the validation evidence' \
            > "$2/server_summary-excluded.txt"
    fi
    return 0
}

REMOTE_DIR=/tmp/server_scheduler_test

case "$FOV_MODE" in
    narrow) FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov_narrow.csv" ;;
    wide)   FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov_wide.csv" ;;
    normal) FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov.csv" ;;
    *)      echo "FoV inválido: $FOV_MODE (use narrow, normal ou wide)" ; exit 1 ;;
esac

if [[ "$NO_BUILD" == 0 ]]; then
    echo -e "${PURPLE}Compiling for Linux...${NC}"

    (cd ../.. && GOOS=linux GOARCH=amd64 go build -o main)
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        exit $EXIT_CODE
    fi

    echo -e "${PURPLE}Uploading to $IP at $REMOTE_DIR...${NC}"

    withSSH "sudo rm -rf $REMOTE_DIR/* && mkdir -p $REMOTE_DIR"
    upload "../../main" "$REMOTE_DIR"
    withSSH "chmod +x $REMOTE_DIR/main"

    # Upload the media as one archive. Sending ~31k small files through one
    # SCP invocation each makes a reproducible validation unnecessarily slow.
    DATA_ARCHIVE=$(mktemp /tmp/tccquic-data-XXXXXX.tar.gz)
    tar -C ../.. -czf "$DATA_ARCHIVE" data
    upload "$DATA_ARCHIVE" "$REMOTE_DIR/data.tar.gz"
    withSSH "cd $REMOTE_DIR && tar xzf data.tar.gz && rm data.tar.gz"
    rm -f "$DATA_ARCHIVE"

    upload "resources/server_scheduler_test.py" "$REMOTE_DIR"
    upload "resources/utils.py" "$REMOTE_DIR"
else
    echo -e "${PURPLE}Skipping build/upload (--no-build)${NC}"
    withSSH "rm -f $REMOTE_DIR/*.csv"
fi

echo -e "${PURPLE}Executing...${NC}"

mkdir -p "$LOG_DIR"
if [[ ! -f "$LOG_DIR/experiment.env" ]]; then
    cat > "$LOG_DIR/experiment.env" <<EOF
server_mode=$SERVER_MODE
abr_mode=$ABR_MODE
server_bw_mbps=$SERVER_BW
client_bw_mbps=$CLIENT_BW
loss_pct=$LOSS
parallelism=$PARALELLISM
delay_ms=$DELAY
background_load_pct=$LOAD
base_latency_ms=$BASE_LATENCY
fov_mode=$FOV_MODE
num_clients=$NUM_CLIENTS
fov_mix=$FOV_MIX
wfq_beta=$WFQ_BETA
BOLA_DEBUG_PATH=$BOLA_DEBUG_PATH
BOLA_QMAX_SEGMENTS=$BOLA_QMAX_SEGMENTS
LEGACY_DEBUG_PATH=$LEGACY_DEBUG_PATH
TEST_CLIENT_SEGMENT_LIMIT=$TEST_CLIENT_SEGMENT_LIMIT
EOF
fi

withSSH "cd $REMOTE_DIR && \
        sudo env SERVER_MODE='$SERVER_MODE' ABR_MODE='$ABR_MODE' SERVER_BW='$SERVER_BW' \
            CLIENT_BW='$CLIENT_BW' LOSS='$LOSS' PARALELLISM='$PARALELLISM' \
            DELAY='$DELAY' LOAD='$LOAD' BASE_LATENCY='$BASE_LATENCY' \
            FOV_TRACE_PATH='$FOV_TRACE_PATH' BOLA_DEBUG_PATH='$BOLA_DEBUG_PATH' \
            BOLA_QMAX_SEGMENTS='$BOLA_QMAX_SEGMENTS' \
            NUM_CLIENTS='$NUM_CLIENTS' FOV_MIX='$FOV_MIX' WFQ_BETA='$WFQ_BETA' \
            LEGACY_DEBUG_PATH='$LEGACY_DEBUG_PATH' TEST_CLIENT_SEGMENT_LIMIT='$TEST_CLIENT_SEGMENT_LIMIT' \
            ./server_scheduler_test.py" 2>&1 | tee "$LOG_DIR/stdout"
PIPE_RC=( "${PIPESTATUS[@]}" )
PY_RC=${PIPE_RC[0]:-1}
TEE_RC=${PIPE_RC[1]:-0}
echo -e "${PURPLE}Exit codes: ssh/python=$PY_RC tee=$TEE_RC${NC}"

download "$REMOTE_DIR/*.csv" "$LOG_DIR" || true

echo -e "${PURPLE}Logs: $(cd "$LOG_DIR" && pwd)${NC}"
exit "$PY_RC"
