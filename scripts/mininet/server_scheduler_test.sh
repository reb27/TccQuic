#!/usr/bin/env bash

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [OPTIONS] <IP>"
    echo "OPTIONS:"
    echo "--fifo, --sp, --wfq     Select server mode (default: fifo)"
    echo "--abr MODE              Select client ABR mode (bola|legacy|fixed|article|article50|article30)"
    echo "--article50             Alias for --abr article50 (modo artigo com 50% alta prioridade)"
    echo "--article30             Alias for --abr article30 (modo artigo com 30% alta prioridade)"
    echo "--scenario N            Apply article scenario preset (1, 3, or 6)"
    echo "--sbw N                 Select server bandwidth in Mbps"
    echo "--cbw N                 Select client bandwidth in Mbps"
    echo "--baselatency N         Select client base latency"
    echo "--loss N                Select loss in %"
    echo "-p N                    Select paralellism"
    echo "--delay N               Select delay"
    echo "--load N                Select load %"
    echo "--fov NAME              Select FoV trace: narrow, normal, wide"
    echo "-o DIR                  Select output directory"
}

SERVER_MODE="wfq"
ABR_MODE="bola"
SERVER_BW="60"
CLIENT_BW="50"
LOSS="4"
PARALELLISM="120"
DELAY="20"
LOAD="30"
BASE_LATENCY="800"
FOV_MODE="normal"
SCENARIO=
IP=
LOG_DIR=
SET_ABR=0
SET_SBW=0
SET_CBW=0
SET_LOSS=0
SET_PAR=0
SET_DELAY=0
SET_LOAD=0

while [[ "$#" > 0 ]]; do
    case "$1" in
    --fifo) SERVER_MODE="fifo"              ; shift   ;;
    --sp)   SERVER_MODE="sp"                ; shift   ;;
    --wfq)  SERVER_MODE="wfq"               ; shift   ;;
    --abr)  ABR_MODE="$2"                   ; SET_ABR=1 ; shift 2 ;;
    --article50) ABR_MODE="article50"       ; SET_ABR=1 ; shift   ;;
    --article30) ABR_MODE="article30"       ; SET_ABR=1 ; shift   ;;
    --scenario) SCENARIO="$2"               ; shift 2 ;;
    --sbw)  SERVER_BW="$2"                  ; SET_SBW=1 ; shift 2 ;;
    --cbw)  CLIENT_BW="$2"                  ; SET_CBW=1 ; shift 2 ;;
    --baselatency)  BASE_LATENCY="$2"       ; shift 2 ;;
    --loss) LOSS="$2"                       ; SET_LOSS=1 ; shift 2 ;;
    -p)     PARALELLISM="$2"                ; SET_PAR=1 ; shift 2 ;;
    --delay) DELAY="$2"                     ; SET_DELAY=1 ; shift 2 ;;
    --load) LOAD="$2"                       ; SET_LOAD=1 ; shift 2 ;;
    --fov)   FOV_MODE="$2"                  ; shift 2 ;;
    -o)     LOG_DIR="$2"                    ; shift 2 ;;
    -*)     showUsage ; exit 1              ; shift   ;;
    *)      IP="$1"                         ; shift   ;;
    esac
done

if [[ -n "$SCENARIO" ]]; then
    case "$SCENARIO" in
        1)
            [[ $SET_DELAY -eq 0 ]] && DELAY="24"
            [[ $SET_LOAD -eq 0 ]] && LOAD="10"
            ;;
        3)
            [[ $SET_DELAY -eq 0 ]] && DELAY="16"
            [[ $SET_LOAD -eq 0 ]] && LOAD="10"
            ;;
        6)
            [[ $SET_DELAY -eq 0 ]] && DELAY="10"
            [[ $SET_LOAD -eq 0 ]] && LOAD="30"
            ;;
        *)
            echo "Scenario inválido: $SCENARIO (use 1, 3 ou 6)"
            exit 1
            ;;
    esac
    [[ $SET_SBW -eq 0 ]] && SERVER_BW="100"
    [[ $SET_CBW -eq 0 ]] && CLIENT_BW="100"
    [[ $SET_LOSS -eq 0 ]] && LOSS="2"
    [[ $SET_PAR -eq 0 ]] && PARALELLISM="120"
    if [[ $SET_ABR -eq 0 ]]; then
        ABR_MODE="article50"
    fi
fi

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
    withSSH "cd $REMOTE_DIR && tar czf results.tar.gz *.csv"
    scp "mininet@$IP:$REMOTE_DIR/results.tar.gz" "$2"
    (cd "$2" && tar xzf results.tar.gz && rm results.tar.gz)
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "scp download failed!"
        echo
        exit $EXIT_CODE
    fi
}

REMOTE_DIR=/tmp/server_scheduler_test

case "$FOV_MODE" in
    narrow) FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov_narrow.csv" ;;
    wide)   FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov_wide.csv" ;;
    normal) FOV_TRACE_PATH="$REMOTE_DIR/data/user_fov.csv" ;;
    *)      echo "FoV inválido: $FOV_MODE (use narrow, normal ou wide)" ; exit 1 ;;
esac

echo -e "${PURPLE}Compiling...${NC}"

(cd ../.. && go build)
EXIT_CODE=$?
if [[ $EXIT_CODE != 0 ]]; then
    exit $EXIT_CODE
fi

echo -e "${PURPLE}Uploading to $IP at $REMOTE_DIR...${NC}"

withSSH "sudo rm -rf $REMOTE_DIR/* && mkdir -p $REMOTE_DIR"
upload "../../main" "$REMOTE_DIR"
withSSH "chmod +x $REMOTE_DIR/main"

upload "../../data" "$REMOTE_DIR"

upload "resources/server_scheduler_test.py" "$REMOTE_DIR"
upload "resources/utils.py" "$REMOTE_DIR"

echo -e "${PURPLE}Executing...${NC}"

mkdir -p "$LOG_DIR"
if [[ ! -f "$LOG_DIR/experiment.env" ]]; then
    : > "$LOG_DIR/experiment.env"
fi
cat >> "$LOG_DIR/experiment.env" <<EOF
scenario=${SCENARIO}
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
EOF

withSSH "cd $REMOTE_DIR && \
        sudo env SERVER_MODE='$SERVER_MODE' ABR_MODE='$ABR_MODE' SERVER_BW='$SERVER_BW' \
            CLIENT_BW='$CLIENT_BW' LOSS='$LOSS' PARALELLISM='$PARALELLISM' \
            DELAY='$DELAY' LOAD='$LOAD' BASE_LATENCY='$BASE_LATENCY' \
            FOV_TRACE_PATH='$FOV_TRACE_PATH' LANG='C.UTF-8' LC_ALL='C.UTF-8' PYTHONIOENCODING='UTF-8' \
            ./server_scheduler_test.py" 2>&1 | tee "$LOG_DIR/stdout"
EXIT_CODE=${PIPESTATUS[0]}
echo -e "${PURPLE}Exit code: $EXIT_CODE${NC}"

download "$REMOTE_DIR/*.csv" "$LOG_DIR"

resources/plot_server_scheduler_test_results.py "$LOG_DIR"/*.csv \
    "$LOG_DIR"

echo -e "${PURPLE}Logs: $(cd "$LOG_DIR" && pwd)${NC}"
