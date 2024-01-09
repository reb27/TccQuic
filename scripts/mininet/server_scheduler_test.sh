#!/usr/bin/env bash

PROGRAM_NAME=$0
showUsage() {
    echo "Usage: $PROGRAM_NAME [OPTIONS] <IP>"
    echo "OPTIONS:"
    echo "--fifo, --sp, --wfq     Select server mode (default: fifo)"
    echo "--sbw N                 Select server bandwidth in Mbps"
    echo "--cbw N                 Select client bandwidth in Mbps"
    echo "--loss N                Select loss in %"
    echo "-p N                    Select paralellism"
    echo "-o DIR                  Select output directory"
}

SERVER_MODE="fifo"
SERVER_BW="100"
CLIENT_BW="100"
LOSS="2"
PARALELLISM="128"
IP=
LOG_DIR=

while [[ "$#" > 0 ]]; do
    case "$1" in
    --fifo) SERVER_MODE="fifo" ; shift   ;;
    --sp)   SERVER_MODE="sp"   ; shift   ;;
    --wfq)  SERVER_MODE="wfq"  ; shift   ;;
    --sbw)  SERVER_BW="$2"     ; shift 2 ;;
    --cbw)  CLIENT_BW="$2"     ; shift 2 ;;
    --loss) LOSS="$2"          ; shift 2 ;;
    -p)     PARALELLISM="$2"   ; shift 2 ;;
    -o)     LOG_DIR="$2"       ; shift 2 ;;
    -*)     showUsage ; exit 1 ; shift   ;;
    *)      IP="$1"            ; shift   ;;
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
            $(basename "${PROGRAM_NAME%.*}") $SERVER_MODE $SERVER_BW \
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
    rsync -a "$1" "mininet@$IP:$2"
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
    rsync -a "mininet@$IP:$1" "$2"
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "scp download failed!"
        echo
        exit $EXIT_CODE
    fi
}

REMOTE_DIR=/tmp/server_scheduler_test

echo -e "${PURPLE}Compiling...${NC}"

(cd ../.. && go build)
EXIT_CODE=$?
if [[ $EXIT_CODE != 0 ]]; then
    exit $EXIT_CODE
fi

echo -e "${PURPLE}Uploading to $IP at $REMOTE_DIR...${NC}"

withSSH "sudo rm -rf $REMOTE_DIR/* && mkdir -p $REMOTE_DIR"
upload "../../main" "$REMOTE_DIR"
upload "../../data" "$REMOTE_DIR"
upload "resources/server_scheduler_test.py" "$REMOTE_DIR"
upload "resources/utils.py" "$REMOTE_DIR"

echo -e "${PURPLE}Executing...${NC}"

mkdir -p "$LOG_DIR"

withSSH "cd $REMOTE_DIR && \
        sudo env SERVER_MODE='$SERVER_MODE' SERVER_BW='$SERVER_BW' \
            CLIENT_BW='$CLIENT_BW' SERVER_LOSS='$LOSS' CLIENT_LOSS='$LOSS' \
            PARALELLISM='$PARALELLISM' \
            ./server_scheduler_test.py" 2>&1 | tee "$LOG_DIR/stdout"
EXIT_CODE=$?
echo -e "${PURPLE}Exit code: $EXIT_CODE${NC}"

download "$REMOTE_DIR/*.csv" "$LOG_DIR"

resources/plot_server_scheduler_test_results.py "$LOG_DIR"/*.csv \
    "$LOG_DIR/figure.png"

echo -e "${PURPLE}Logs: $(cd "$LOG_DIR" && pwd)${NC}"
