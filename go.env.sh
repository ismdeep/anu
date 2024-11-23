#!/usr/bin/env bash

# Usage: bash go.env.sh <go-version>
#    <go-version> example:
#      - go1.20.12

go_version="${1:?}" && \
target_binary_root="${HOME}/.x98/runtime/${go_version}" && \
if [ ! -f "${target_binary_root}/bin/go" ]; then
  echo "${go_version} is installing ..." && \
  # get file name
  file_name="" && \
  case $(uname -s)-$(uname -m) in
    Linux-x86_64   | Linux-amd64)  file_name="${go_version}.linux-amd64.tar.gz"  ;;
    Linux-aarch64  | Linux-arm64)  file_name="${go_version}.linux-arm64.tar.gz"  ;;
    Darwin-x86_64  | Darwin-amd64) file_name="${go_version}.darwin-amd64.tar.gz" ;;
    Darwin-aarch64 | Darwin-arm64) file_name="${go_version}.darwin-arm64.tar.gz" ;;
    *) echo "ERROR: unsupported platform [$(uname -s)-$(uname -m)]" && return 1 ;;
  esac && \
  # download
  tmpdir="${HOME}/.x98/tmp/go-download-$(openssl rand -hex 32)" && \
  download_url="https://doraemon.uniontech.com/dist/library/go/${go_version}/${file_name}" && \
  curl -fL "${download_url}" --create-dirs -o "${tmpdir}/${file_name}" && \
  # extract
  (
    cd "${tmpdir}" && \
    tar -zxf "${file_name}" && \
    mv go/   "${go_version}/"
  ) && \
  mkdir -p                            "${target_binary_root}/" && \
  rsync -a "${tmpdir}/${go_version}/" "${target_binary_root}/" && \
  # clean
  rm -rf "${tmpdir}"
fi && \
export GOPRIVATE=pkg.deepin.com && \
export GOPROXY=https://goproxy.cn,direct && \
export GOROOT="${target_binary_root}" && \
export GOPATH="${HOME}/.x98/var/lib/gopath" && \
mkdir -p "${GOPATH}" && \
export PATH="${GOROOT}/bin:${PATH}"