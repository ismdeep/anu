#!/usr/bin/env bash

set -e

# Get to workdir
cd "$(realpath "$(dirname "$(realpath "${BASH_SOURCE[0]}")")")"

# load go env
source "go.env.sh" "go1.20.13"

# Git 提交时间
COMMIT_TIME=$(TZ=Asia/Shanghai git log -1 --format="%cd" --date=format:"%Y%m%d%H%M%S")

# prepare version info
version="$(cat version/VERSION)+${COMMIT_TIME}"

# build
build-binary() {
  os="${1:?}" && \
  arch="${2:?}" && \
  GCC_NAME="" && \
  case ${arch} in
    amd64) GCC_NAME=x86_64-linux-gnu-gcc ;;
    arm64) GCC_NAME=aarch64-linux-gnu-gcc ;;
  esac && \
  file_name="anu_${version}_${os}_${arch}" && \
  # please set CGO_ENABLED=0
  GOOS=${os} GOARCH=${arch} CGO_ENABLED=0 CC=${GCC_NAME} \
    go build \
      -o "output/${file_name}" \
      -trimpath \
      -ldflags '-s -w' \
      github.com/ismdeep/anu && \
  echo "OK: output/${file_name}"
}

# build deb
build-deb() {
  arch=${1:?} && \
  deb_arch="${arch}" && \
  mkdir -p "debian/anu/DEBIAN/" && \
  installed_size="$(du -ks "debian/anu/" | cut -f 1)" && \
  mkdir -p "debian/anu/usr/bin/" && \
  rsync -a "output/anu_${version}_linux_${arch}" "debian/anu/usr/bin/anu" && \
  chmod +x "debian/anu/usr/bin/anu" && \
  ANU_VERSION=${version} ANU_ARCH=${deb_arch} INSTALLED_SIZE=${installed_size} \
    envsubst < "debian/control.tpl" > "debian/anu/DEBIAN/control" && \
  file_name="anu_${version}_${deb_arch}.deb"
  dpkg  -b "debian/anu" "output/deb/${file_name}" >/dev/null 2>&1 && \
  echo "OK: output/deb/${file_name}"
}

rm -r -f output/ && echo 'Directory removed: output/'
mkdir -p output/ && echo 'Directory created: output/'

build-binary linux  amd64
build-binary linux  arm64
build-binary darwin amd64
build-binary darwin arm64

mkdir -p output/deb/ && echo 'Directory created: output/deb/'
build-deb          amd64
build-deb          arm64
