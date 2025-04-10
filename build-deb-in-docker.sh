#!/usr/bin/env bash

set -e
set -u
set -o pipefail

log_debug() {
  echo -e "$(date '+%Y-%m-%d %H:%M:%S %z') [ \033[34mDEBUG\033[0m ] $*"
}

log_info() {
  echo -e "$(date '+%Y-%m-%d %H:%M:%S %z') [ \033[36mINFO\033[0m ] $*"
}

log_success() {
  echo -e "$(date '+%Y-%m-%d %H:%M:%S %z') [ \033[32mOK\033[0m ] $*"
}

log_error() {
  echo -e "$(date '+%Y-%m-%d %H:%M:%S %z') [ \033[31mERROR\033[0m ] $*" >&2
}

# Get to workdir
cd "$(realpath "$(dirname "$(realpath "${BASH_SOURCE[0]}")")")"

# Get arch
arch="${1:?}"

# 获取版本号（根据 debian/changelog）
version=$(< debian/changelog head -n 1 | awk '{print $2}' | sed 's/[()]//g')
log_info "版本号: ${version}"

log_info "Creating docker container (${arch}) ..."
if [ "$(docker ps | grep -c "anu-deb-builder-${arch}")" == "0" ]; then
  docker run \
    --rm \
    --platform "linux/${arch}" \
    --name "anu-deb-builder-${arch}" \
    -d hub.deepin.com/public/uniteos:2021 sleep infinity
fi
log_success "Docker container created."

log_info "[INFO] Building deb package via Docker..."
docker exec "anu-deb-builder-${arch}" \
  apt-get update
docker exec "anu-deb-builder-${arch}" \
  apt-get install -y build-essential make curl rsync git gettext-base debhelper-compat
log_success "basic packages installed."

docker exec "anu-deb-builder-${arch}" \
  rm -rf /anu/
docker cp . "anu-deb-builder-${arch}":/anu/
log_success "code copied."

docker exec "anu-deb-builder-${arch}" \
  git config --global --add safe.directory /anu
log_success "git config safe.directory set."

docker exec --workdir /anu "anu-deb-builder-${arch}" \
  dpkg-buildpackage -uc -us
log_success "deb package built."

mkdir -p ./output/
docker cp "anu-deb-builder-${arch}:/anu_${version}_${arch}.deb" ./output/
log_success "deb package anu_${version}_${arch}.deb copied to output/"