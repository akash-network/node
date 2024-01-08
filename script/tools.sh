#!/usr/bin/env bash

set -x

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SEMVER=$SCRIPT_DIR/semver.sh

gomod="$SCRIPT_DIR/../go.mod"

function get_gotoolchain() {
    local gotoolchain
    local goversion
    local local_goversion

    gotoolchain=$(grep -E '^toolchain go[0-9]{1,}.[0-9]{1,}.[0-9]{1,}$' < "$gomod" | cut -d ' ' -f 2 | tr -d '\n')
    goversion=$(grep -E '^go [0-9]{1,}.[0-9]{1,}(.[0-9]{1,})?$' < "$gomod" | cut -d ' ' -f 2 | tr -d '\n')

    if [[ ${gotoolchain} == "" ]]; then
        # determine go toolchain from go version in go.mod
        if which go > /dev/null 2>&1 ; then
            local_goversion=$(GOTOOLCHAIN=local go version | cut -d ' ' -f 3 | sed 's/go*//' | tr -d '\n')
            if [[ $($SEMVER compare "v$local_goversion" v"$goversion") -ge 0 ]]; then
                goversion=$local_goversion
            else
                local_goversion=
            fi
        fi

        if [[ "$local_goversion" == "" ]]; then
            goversion=$(curl -s "https://go.dev/dl/?mode=json&include=all" | jq -r --arg regexp "^go$goversion" '.[] | select(.stable == true) | select(.version | match($regexp)) | .version' | head -n 1 |  sed -e s/^go//)
        fi

        if [[ $goversion != "" ]] && [[ $($SEMVER compare "v$goversion" v1.21.0) -ge 0 ]]; then
            gotoolchain=go${goversion}
        else
            gotoolchain=go$(grep -E '^go [0-9]{1,}.[0-9]{1,}$' < "$gomod" | cut -d ' ' -f 2 | tr -d '\n').0
        fi
    fi

    echo -n "$gotoolchain"
}

case "$1" in
gotoolchain)
    get_gotoolchain
    ;;
esac
