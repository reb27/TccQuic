#!/usr/bin/env bash

# Testsuite for the different scenarios

set -e

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
        $(basename "${PROGRAM_NAME%.*}") $LOG_NUMBER)
    if [[ ! -e "$SUPER_LOG_DIR" ]]; then
        break
    fi
    LOG_NUMBER=$((LOG_NUMBER+1))
done

# Environment variables:
# SCENARIO
# BW
# DELAY
# LOAD
launchTest() {
    PARALELLISM=120
    LOSS=2
    for BASE_LATENCY in 10 50 100 200 300 400 500 1000; do
        for MODE in fifo sp wfq; do
            LOG_DIR=$(LC_NUMERIC="en_US.UTF-8" \
                printf "%s/scenario%d-baselatency%d/%s/" \
                $SUPER_LOG_DIR $SCENARIO $BASE_LATENCY $MODE)
            mkdir -p "$LOG_DIR"
            PARAMS=(-o $LOG_DIR --$MODE --sbw $BW --cbw $BW \
                --loss $LOSS -p $PARALELLISM --delay $DELAY --load $LOAD \
                --baselatency $BASE_LATENCY)
            echo "${PARAMS[@]}" > "$LOG_DIR/parameters"
            ./server_scheduler_test.sh "${PARAMS[@]}" "$IP"
        done
    done
}

SCENARIO=1
LOAD=10
BW=100
DELAY=24
launchTest

SCENARIO=2
LOAD=30
BW=100
DELAY=24
launchTest

SCENARIO=3
LOAD=10
BW=100
DELAY=16
launchTest

SCENARIO=4
LOAD=30
BW=100
DELAY=16
launchTest

SCENARIO=5
LOAD=10
BW=100
DELAY=10
launchTest

SCENARIO=6
LOAD=30
BW=100
DELAY=10
launchTest
