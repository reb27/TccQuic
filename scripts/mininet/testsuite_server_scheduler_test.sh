#!/usr/bin/env bash

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

for SBW in 10 100; do
    for CBW in 1 10 100; do
        for LOSS in 00 01 10; do
            for M in fifo sp wfq; do
                LOG_DIR=$(LC_NUMERIC="en_US.UTF-8" \
                    printf "%s/sbw%s-cbw%s-loss%s/%s/" \
                    $SUPER_LOG_DIR $SBW $CBW $LOSS $M)
                ./server_scheduler_test.sh -o $LOG_DIR \
                    --$M --sbw $SBW --cbw $CBW --loss $LOSS "$IP"
            done
        done
    done
done
