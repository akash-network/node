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

case "$1" in
test-required)
    shift
    curr_ref=$1

    upgrades_dir=${ROOT_DIR}/upgrades/software
    upgrade_name=$(find "${upgrades_dir}" -mindepth 1 -maxdepth 1 -type d | awk -F/ '{print $NF}' | sort -r | head -n 1)

    meta=$(cat "${ROOT_DIR}/meta.json")
    # upgrade under test is always highest semver in the upgrades list
    #uut=$(echo "$meta" | jq -er '.upgrades | keys | .[]' | sort -r | head -n 1)

    # shellcheck disable=SC2086
    $semver validate $upgrade_name

    # current git reference is matching upgrade name. looks like release has been cut
    # so lets run the last test
    if [[ "$curr_ref" == "$upgrade_name" ]]; then
        echo -e "true"
        exit 0
    fi

    cnt=0
    while :; do
        cnt=$((cnt+1))
        if [[ $cnt -gt 100 ]];then
            echoerr "unable to determine tag to test upgrade"
            exit 1
        fi

        # shellcheck disable=SC2086
        if git tag -v $upgrade_name >/dev/null 2>&1; then
            if echo "$meta" | jq -e --arg name $upgrade_name '.revoked_releases[] | contains($name)' >/dev/null 2>&1; then
                $semver bump patch $upgrade_name
                upgrade_name="v$upgrade_name"
            else
                upgrade_name=""
                break
            fi
        else
            break
        fi
    done

    if [[ "$upgrade_name" == "" ]]; then
        echo -n "false"
    else
        echo -n "true"
    fi

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
