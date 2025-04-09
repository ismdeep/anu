#!/usr/bin/env bash

set -e

# Get to workdir
cd "$(realpath "$(dirname "$(realpath "${BASH_SOURCE[0]}")")")"

# load go env
source "go.env.sh" "go1.23.6"

mkdir -p ./deb/usr/bin/
go build -o ./deb/usr/bin/anu -mod vendor -trimpath -ldflags '-s -w' github.com/ismdeep/anu
echo "$(date -R) [INFO] anu binary build completed."