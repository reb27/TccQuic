#!/usr/bin/env bash

# cd to script directory
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

DIRECTORIES=(
    "server/uml"
)

generateUml() {
    for DIR in "${DIRECTORIES[@]}"; do
        (cd "$DIR" && plantuml *.puml)
    done
}

clean() {
    for DIR in "${DIRECTORIES[@]}"; do
        (cd "$DIR" && rm -f *.png)
    done
}

showUsage() {
    echo "Usage: $0 [-c]"
    exit 1
}

if [ $# == 0 ]; then
    generateUml
elif [ $# == 1 ] && [ "$1" == "-c" ]; then
    clean
else
    showUsage
fi
