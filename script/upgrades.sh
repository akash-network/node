#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
ROOT_DIR=$(realpath "${SCRIPT_DIR}"/../)

semver=$(printf %q "${SCRIPT_DIR}/semver.sh")

PROG=upgrades.sh

USAGE="\
Usage:
  $PROG test-required <current reference>
  $PROG --help
Options:
  -h, --help             Print this help message.
Commands:
  test-required  Determine if latest present upgrade needed test run.
                 Conditions to run test:
                  - If current reference matches last upgrade in a codebase
                  - If the codebase has tag matching to the upgrade name, but release is marked as revoked
                  - If the codebase does not have tag matching upgrade name
                 Exit codes:
                  - 0 test required
                  - 1 something went wrong. check stderr"

echoerr() { echo "$@" 1>&2; }

GENESIS_BINARY_VERSION=${UTEST_GENESIS_BINARY_VERSION:=}

WORKDIR=${UTEST_WORKDIR:=}
UPGRADE_FROM=${UTEST_UPGRADE_FROM:=}
UPGRADE_TO=${UTEST_UPGRADE_TO:=}
CONFIG_FILE=${UTEST_CONFIG_FILE:=}

short_opts=h
long_opts=help/workdir:/ufrom:/uto:/gbv:/config: # those who take an arg END with :

while getopts ":$short_opts-:" o; do
    case $o in
        :)
            echo >&2 "option -$OPTARG needs an argument"
            continue
            ;;
        '?')
            echo >&2 "bad option -$OPTARG"
            continue
            ;;
        -)
            o=${OPTARG%%=*}
            OPTARG=${OPTARG#"$o"}
            lo=/$long_opts/
            case $lo in
                *"/$o"[!/:]*"/$o"[!/:]*)
                    echo >&2 "ambiguous option --$o"
                    continue
                    ;;
                *"/$o"[:/]*)
                    ;;
                *)
                    o=$o${lo#*"/$o"};
                    o=${o%%[/:]*}
                    ;;
            esac

            case $lo in
                *"/$o/"*)
                    OPTARG=
                    ;;
                *"/$o:/"*)
                    case $OPTARG in
                        '='*)
                            OPTARG=${OPTARG#=}
                            ;;
                        *)
                            eval "OPTARG=\$$OPTIND"
                            if [ "$OPTIND" -le "$#" ] && [ "$OPTARG" != -- ]; then
                                OPTIND=$((OPTIND + 1))
                            else
                                echo >&2 "option --$o needs an argument"
                                continue
                            fi
                            ;;
                    esac
                    ;;
            *) echo >&2 "unknown option --$o"; continue;;
            esac
    esac
    case "$o" in
        workdir)
            WORKDIR=$OPTARG
            ;;
        ufrom)
            UPGRADE_FROM=$OPTARG
            ;;
        uto)
            UPGRADE_TO=$OPTARG
            ;;
        gbv)
            GENESIS_BINARY_VERSION=$OPTARG
            ;;
        config)
            CONFIG_FILE=$OPTARG
            ;;
    esac
done
shift "$((OPTIND - 1))"

GENESIS_ORIG=${UTEST_GENESIS_ORIGIN:=https://github.com/akash-network/testnetify/releases/download/${UPGRADE_FROM}/genesis.json.tar.lz4}

function content_type() {
    case "$1" in
        *.tar.cz*)
            tar_cmd="tar -xJ -"
            ;;
        *.tar.gz*)
            tar_cmd="tar xzf -"
            ;;
        *.tar.lz4*)
            tar_cmd="lz4 -d | tar xf -"
            ;;
        *.tar.zst*)
            tar_cmd="zstd -cd | tar xf -"
            ;;
        *)
            tar_cmd="tar xf -"
            ;;
    esac

    echo "$tar_cmd"
}

function content_size() {
    local size_in_bytes

    size_in_bytes=$(wget "$1" --spider --server-response -O - 2>&1 | grep "Content-Length" | awk '{print $2}' | tr -d '\n')
    err=$?
    case "$size_in_bytes" in
        # Value cannot be started with `0`, and must be integer
    [1-9]*[0-9])
        echo "$size_in_bytes"
        ;;
    esac

    return "$err"
}

function content_name() {
    name=$(wget "$1" --spider --server-response -O - 2>&1 | grep "Content-Disposition:" | tail -1 | awk -F"filename=" '{print $2}')
    # shellcheck disable=SC2181
    if [[ "$name" == "" ]]; then
        echo "$1"
    else
        echo "$name"
    fi
}

uname_arch() {
    arch=$(uname -m)
    case $arch in
        x86_64) arch="amd64" ;;
        x86) arch="386" ;;
        i686) arch="386" ;;
        i386) arch="386" ;;
        aarch64) arch="arm64" ;;
        armv5*) arch="armv5" ;;
        armv6*) arch="armv6" ;;
        armv7*) arch="armv7" ;;
    esac
    echo "${arch}"
}

untar() {
    tarball=$1
    case "${tarball}" in
        *.tar.gz | *.tgz) tar -xzf "${tarball}" ;;
        *.tar) tar -xf "${tarball}" ;;
        *.zip) unzip "${tarball}" ;;
        *)
            log_err "untar unknown archive format for ${tarball}"
            return 1
            ;;
    esac
}

function init() {
    if [[ -z "${WORKDIR}" ]]; then
        echo "workdir is not set"
        echo -e "$USAGE";
        exit 1
    fi

    local config
    config=$(cat "$CONFIG_FILE")

    local cnt=0
    local validators_dir=${WORKDIR}/validators

    mkdir -p "${WORKDIR}/validators/logs"

    for val in $(jq -c '.validators[]' <<<"$config"); do
        local valdir=$validators_dir/.akash${cnt}
        local cosmovisor_dir=$valdir/cosmovisor
        local genesis_bin=$cosmovisor_dir/genesis/bin
        local upgrade_bin=$cosmovisor_dir/upgrades/$UPGRADE_TO/bin

        local AKASH=$genesis_bin/akash

        mkdir -p "$genesis_bin"
        mkdir -p "$upgrade_bin"

        if [[ $cnt -eq 0 ]]; then
            "$ROOT_DIR"/install.sh -b "$genesis_bin" "$GENESIS_BINARY_VERSION"

            AKASH=$upgrade_bin/akash make -sC "$ROOT_DIR" akash
        else
            cp "$validators_dir/.akash0/cosmovisor/genesis/bin/akash" "$genesis_bin/akash"
            cp "$validators_dir/.akash0/cosmovisor/upgrades/$UPGRADE_TO/bin/akash" "$upgrade_bin/akash"
        fi

        $AKASH init --home "$valdir" "$(jq -rc '.moniker' <<<"$val")" > /dev/null 2>&1

        if [[ $cnt -eq 0 ]]; then
            pushd "$(pwd)"
            cd "$valdir/config"

            if [[ "${GENESIS_ORIG}" =~ ^https?:\/\/.* ]]; then
                echo "Downloading genesis from $GENESIS_ORIG"
                wget -qO - "$GENESIS_ORIG" | lz4 - -d | tar xf - -C "$valdir/config"

                pv_args="-petrafb -i 5"
                sz=$(content_size "$GENESIS_ORIG")
                # shellcheck disable=SC2181
                if [ $? -eq 0 ]; then
                    if [[ -n $sz ]]; then
                        pv_args+=" -s $sz"
                    fi

                    tar_cmd=$(content_type "$(content_name "$GENESIS_ORIG")")

                    # shellcheck disable=SC2086
                    (wget -nv -O - "$GENESIS_ORIG" | pv $pv_args | eval " $tar_cmd") 2>&1 | stdbuf -o0 tr '\r' '\n'
                else
                    echo "unable to download genesis"
                fi
            else
                echo "Unpacking genesis from $GENESIS_ORIG"
                tar_cmd=$(content_type "$GENESIS_ORIG")
                # shellcheck disable=SC2086
                (pv -petrafb -i 5 "$GENESIS_ORIG" | eval "$tar_cmd") 2>&1 | stdbuf -o0 tr '\r' '\n'
            fi

            popd

            jq -c '.mnemonics[]' <<<"$config" | while read -r mnemonic; do
                jq -c '.keys[]' <<<"$mnemonic" | while read -r key; do
                    jq -rc '.phrase' <<<"$mnemonic" | $AKASH --home="$valdir" --keyring-backend=test keys add "$(jq -rc '.name' <<<"$key")" --recover --index "$(jq -rc '.index' <<<"$key")"
                done
            done
        else
            cp -r "$validators_dir/.akash0/config/genesis.json" "$valdir/config/genesis.json"
        fi

        jq -r '.keys.priv' <<< "$val" > "$valdir/config/priv_validator_key.json"
        jq -r '.keys.node' <<< "$val" > "$valdir/config/priv_validator_key.json"

        ((cnt++)) || true
    done
}

function clean() {
    if [[ -z "${WORKDIR}" ]]; then
        echo "workdir is not set"
        echo -e "$USAGE";
        exit 1
    fi

    local config
    config=$(cat "$CONFIG_FILE")

    local cnt=0
    local validators_dir=${WORKDIR}/validators

    for val in $(jq -c '.validators[]' <<<"$config"); do
        local valdir=$validators_dir/.akash${cnt}
        local cosmovisor_dir=$valdir/cosmovisor

        rm -rf "$validators_dir/logs/.akash${cnt}-stderr.log"
        rm -rf "$validators_dir/logs/.akash${cnt}-stdout.log"

        rm -rf "$valdir"/data/*
        rm -rf "$cosmovisor_dir/current"
        rm -rf "$cosmovisor_dir/upgrades/${UPGRADE_TO}/upgrade-info.json"

        echo '{"height":"0","round": 0,"step": 0}' > "$valdir/data/priv_validator_state.json"

        ((cnt++)) || true
    done
}

case "$1" in
init)
    shift
    init
    ;;
clean)
    shift
    clean
    ;;
test-required)
    shift
    curr_ref=$1

    upgrades_dir=${ROOT_DIR}/upgrades/software
    upgrade_name=$(find "${upgrades_dir}" -mindepth 1 -maxdepth 1 -type d | awk -F/ '{print $NF}' | sort -r | head -n 1)

    # shellcheck disable=SC2086
    $semver validate $upgrade_name

    # current git reference is matching upgrade name. looks like release has been cut
    # so lets run the last test
    if [[ "$curr_ref" == "$upgrade_name" ]]; then
        echo -e "$upgrade_name"
        exit 0
    fi

    cnt=0

    retracted_versions=$(go mod edit --json | jq -cr .Retract)

    while :; do
        cnt=$((cnt+1))
        if [[ $cnt -gt 100 ]];then
            echoerr "unable to determine tag to test upgrade"
            exit 1
        fi

        # shellcheck disable=SC2086
        if git show-ref --tags $upgrade_name >/dev/null 2>&1; then
            is_retracted=false
            for retracted in $(jq -c '.[]' <<<"$retracted_versions"); do
                vLow=$(jq -rc '.Low' <<<"$retracted")
                vHigh=$(jq -rc '.High' <<<"$retracted")
                tagsAreEqual=$($semver compare $vLow $vHigh)

                isTagInHigh=$($semver compare $upgrade_name $vHigh)
                if [[ $isTagInHigh -le 0 ]]; then
                    if [[ $isTagInHigh -eq 0 ]]; then
                        is_retracted=true
                        break
                    elif [[ $tagsAreEqual -ne 0 ]]; then
                        isTagInLow=$($semver compare $upgrade_name $vLow)
                        if [[ $isTagInLow -ge 0 ]]; then
                            upgrade_name=$vHigh
                            is_retracted=true
                            break
                        fi
                    fi
                fi
            done

            if [[ $is_retracted == "true" ]]; then
                upgrade_name=v$($semver bump patch $upgrade_name)
            else
                upgrade_name=""
                break
            fi
        else
            break
        fi
    done

    echo -n "$upgrade_name"

    exit 0
    ;;
--help | -h)
    echo -e "$USAGE";
    exit 0
    ;;
*)
    echo "unknown command $1"
    echo -e "$USAGE";
    exit 1
    ;;
esac
