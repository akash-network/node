#!/usr/bin/env bash

# in akash even minor part of the tag indicates release belongs to the MAINNET
# using it as scripts simplifies debugging as well as portability
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

if [[ $# -ne 1 ]]; then
	echo "illegal number of parameters"
	exit 1
fi

[[ $(($("${SCRIPT_DIR}"/semver.sh get minor "$1") % 2)) -eq 0 ]] && exit 0 && exit 1
