#!/usr/bin/env bash

set -e

#################### CONFIG ####################
publish_target='root@10.0.33.50:/data/build/iup-tool-repo/iup-tool-raw/develop/'
#################### CONFIG ####################

# Get to workdir
cd "$(realpath "$(dirname "$(realpath "${BASH_SOURCE[0]}")")")"

rsync -a -r --no-i-r --info=progress2 --info=name0 --no-owner --no-group --no-perms \
  ./output/deb/ \
  "${publish_target}"

ssh root@10.0.33.50 'cd /data/build/iup-tool-repo/ && make build && make publish'

echo "PUBLISHED TO ${publish_target}"
