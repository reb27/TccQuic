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

mkdir -p "$SUPER_LOG_DIR"

for SBW in 100; do
    for CBW in 100; do
        for LOSS in 2; do
            for PARALELLISM in 1 2 4 8 16 32 64 128; do
                for M in fifo sp wfq; do
                    LOG_DIR=$(LC_NUMERIC="en_US.UTF-8" \
                        printf "%s/sbw%s-cbw%s-loss%s-parallelism%d/%s/" \
                        $SUPER_LOG_DIR $SBW $CBW $LOSS $PARALELLISM $M)
                    ./server_scheduler_test.sh -o $LOG_DIR \
                        --$M --sbw $SBW --cbw $CBW --loss $LOSS \
                        -p $PARALELLISM "$IP"
                done
            done
        done
    done
done
