#!/usr/bin/env bash

set -e

dtag="$(git log -1 --format="%cd" --date=format:'%Y%m%d%H%M%S')-$(git rev-parse HEAD | cut -c1-12)"

output_dir="output/binary/${dtag}"

rm -rf "${output_dir:?}/"
mkdir -p "${output_dir:?}/"

GOOS=linux  GOARCH=amd64 go build -o "${output_dir:?}/anu_linux_amd64"  -trimpath -ldflags '-s -w' .
GOOS=linux  GOARCH=arm64 go build -o "${output_dir:?}/anu_linux_arm64"  -trimpath -ldflags '-s -w' .
GOOS=darwin GOARCH=amd64 go build -o "${output_dir:?}/anu_darwin_amd64" -trimpath -ldflags '-s -w' .
GOOS=darwin GOARCH=arm64 go build -o "${output_dir:?}/anu_darwin_arm64" -trimpath -ldflags '-s -w' .
