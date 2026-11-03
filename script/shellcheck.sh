#!/bin/bash

set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

find "$root" \
    -type d \( -name .git -o -name .cache -o -name node_modules -o -name vendor \) -prune -o \
    -type f -name '*.sh' ! -name godownloader.sh \
    -exec shellcheck -x --source-path=SCRIPTDIR --format=gcc --exclude=SC1091 {} +
