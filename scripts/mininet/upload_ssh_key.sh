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

PRIVATE_KEY=~/.ssh/id_ecdsa
if [[ ! -f "$PRIVATE_KEY" ]]; then
    echo "File $PRIVATE_KEY does not exist. The key will be created."
    echo "If you wish to use another algorithm such as RSA, please press N"\
         "and upload it without using this script."
    read -p "Continue? [y/N] " RESPONSE
    echo

    case $RESPONSE in
        [YySs])
            ;;
        *)
            exit 1
    esac

    ssh-keygen -q -f "$PRIVATE_KEY" -t ecdsa

    echo
fi

ssh-copy-id -i "$PRIVATE_KEY" "mininet@$IP"
