#!/usr/bin/env bash

PROGRAM_NAME=$0
showUsage() {
    echo "Uso: $PROGRAM_NAME <IP>"
}

if [[ "$#" != 1 ]]; then
    showUsage
    exit 1
fi

IP=$1

############################################################################

PURPLE='\033[0;35m'
NC='\033[0m'

cd -- "$( dirname -- "${BASH_SOURCE[0]}" )"

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
    scp -q -B "$1" "mininet@$IP:$2"
    EXIT_CODE=$?
    if [[ $EXIT_CODE != 0 ]]; then
        echo
        echo "scp upload failed!"
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
upload "resources/server_scheduler_test.py" "$REMOTE_DIR"
upload "resources/utils.py" "$REMOTE_DIR"

echo -e "${PURPLE}Executing...${NC}"

withSSH "chmod +x $REMOTE_DIR/server_scheduler_test.py && \
         sudo $REMOTE_DIR/server_scheduler_test.py"
EXIT_CODE=$?
if [[ $EXIT_CODE != 0 ]]; then
    exit $EXIT_CODE
fi
