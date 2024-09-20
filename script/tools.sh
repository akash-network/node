#!/usr/bin/env bash

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

replace_paths() {
    local file="${1}"
    local cimport="${2}"
    local nimport="${3}"
    local sedcmd=sed

    if [[ "$OSTYPE" == "darwin"* ]]; then
        sedcmd=gsed
    fi

    $sedcmd -ri "s~$cimport~$nimport~" "${file}"
}

function replace_import_path() {
    local next_major_version=$1
    local curr_module_name
    local curr_version
    local new_module_name

    curr_module_name=$(cd go || exit; go list -m)
    curr_version=$(echo "$curr_module_name" | sed -n 's/.*v\([0-9]*\).*/\1/p')
    new_module_name=${curr_module_name%/"v$curr_version"}/$next_major_version

    echo "current import paths are $curr_module_name, replacing with $new_module_name"

    declare -a modules_to_upgrade_manually

    modules_to_upgrade_manually+=("./go/go.mod")

    echo "preparing files to replace"

    declare -a files

    while IFS= read -r line; do
        files+=("$line")
    done < <(find . -type f -not \( \
            -path "./install.sh" \
        -or -path "./upgrades/*" \
        -or -path "./.cache/*" \
        -or -path "./dist/*" \
        -or -path "./.git*" \
        -or -name "*.md" \
        -or -path "./.idea/*" \))

    echo "updating all files"

    for file in "${files[@]}"; do
        if test -f "$file"; then
            # skip files that need manual upgrading
            for excluded_file in "${modules_to_upgrade_manually[@]}"; do
                if [[ "$file" == *"$excluded_file"* ]]; then
                    continue 2
                fi
            done

            replace_paths "$file" "\"$curr_module_name" "\"$new_module_name"
        fi
    done

    echo "updating go.mod"
    for retract in $(cd go || exit; go mod edit --json | jq -cr '.Retract | if . != null then .[] else empty end'); do
        local low
        local high

        low=$(jq -r '.Low' <<<"$retract")
        high=$(jq -r '.High' <<<"$retract")
        echo "    dropping retract: [$low, $high]"
        go mod edit -dropretract=["$low","$high"]
    done

    replace_paths "./go/go.mod" "$curr_module_name" "$new_module_name"
}

case "$1" in
gotoolchain)
    get_gotoolchain
    ;;
replace-import-path)
    shift
    replace_import_path "$@"
    ;;
esac
