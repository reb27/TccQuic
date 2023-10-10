#!/usr/bin/env bash

# cd to script directory
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

if [ $# == 0 ]; then
    # call plantuml
    plantuml *.puml
elif [ $# == 1 ] && [ "$1" == "-c" ]; then
    rm -f *.png
else
    echo "Usage: $0 [-c]"
    exit 1
fi
