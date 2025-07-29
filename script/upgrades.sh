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
CHAIN_METADATA_URL=https://raw.githubusercontent.com/akash-network/net/master/mainnet/meta.json
SNAPSHOT_URL=https://snapshots.akash.network/akashnet-2/akashnet-2_22503451.tar.lz4
STATE_CONFIG=

short_opts=h
long_opts=help/workdir:/ufrom:/uto:/gbv:/config:/chain-meta:/snapshot-url:/state-config: # those who take an arg END with :

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
				*"/$o"[:/]*) ;;

				*)
					o=$o${lo#*"/$o"}
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
				*)
					echo >&2 "unknown option --$o"
					continue
					;;
			esac
			;;
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
		chain-meta)
			CHAIN_METADATA_URL=$OPTARG
			;;
		snapshot-url)
			SNAPSHOT_URL=$OPTARG
			;;
		state-config)
			STATE_CONFIG=$OPTARG
			;;
	esac
done
shift "$((OPTIND - 1))"

CHAIN_METADATA=$(curl -s "${CHAIN_METADATA_URL}")
GENESIS_URL="$(echo "$CHAIN_METADATA" | jq -r '.codebase.genesis.genesis_url? // .genesis?')"

pushd() {
	command pushd "$@" >/dev/null
}

popd() {
	command popd >/dev/null
}

function tar_by_content_type() {
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

function content_type() {
	case "$1" in
		*.tar.cz*)
			tar_cmd="tar.cz"
			;;
		*.tar.gz*)
			tar_cmd="tar.gz"
			;;
		*.tar.lz4*)
			tar_cmd="tar.lz4"
			;;
		*.tar.zst*)
			tar_cmd="tar.zst"
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
	name=$(wget "$1" --spider --server-response -O - 2>&1 | grep -i "content-disposition" | awk -F"filename=" '{print $2}')
	# shellcheck disable=SC2181
	if [[ "$name" == "" ]]; then
		echo "$1"
	else
		echo "$name"
	fi
}

function content_location() {
	name=$(wget "$1" --spider --server-response -O - 2>&1 | grep "location:" | awk '{print $2}' | tr -d '\n')
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

function build_bins() {
	local genesis_bin
	local upgrade_bin

	genesis_bin=$1
	upgrade_bin=$2

	ARCH=$GOARCH OS=$GOOS "$ROOT_DIR"/install.sh -b "$genesis_bin" "$GENESIS_BINARY_VERSION"

	make -sC "$ROOT_DIR" test-bins

	local archive
	archive="akash"

	if [[ $GOOS == "darwin" ]]; then
		archive="${archive}_darwin_all"
	else
		archive="${archive}_linux_$GOARCH"
	fi

	unzip -o "${AKASH_DEVCACHE}/goreleaser/test-bins/${archive}.zip" -d "$upgrade_bin"
	chmod +x "$genesis_bin/akash"
	chmod +x "$upgrade_bin/akash"
}

function init() {
	if [[ -z "${WORKDIR}" ]]; then
		echo "workdir is not set"
		echo -e "$USAGE"
		exit 1
	fi

	local config
	config=$(cat "$CONFIG_FILE")

	local cnt=0
	local validators_dir=${WORKDIR}/validators

	mkdir -p "${WORKDIR}/validators/logs"

	snapshot_file=${validators_dir}/snapshot

	for val in $(jq -c '.validators[]' <<<"$config"); do
		local valdir
		local cosmovisor_dir
		local genesis_bin
		local upgrade_bin
		local AKASH

		valdir=$validators_dir/.akash${cnt}
		cosmovisor_dir=$valdir/cosmovisor
		genesis_bin=$cosmovisor_dir/genesis/bin
		upgrade_bin=$cosmovisor_dir/upgrades/$UPGRADE_TO/bin

		mkdir -p "$genesis_bin"
		mkdir -p "$upgrade_bin"

		if [[ $cnt -eq 0 ]]; then
			build_bins "$genesis_bin" "$upgrade_bin"
		else
			cp "$validators_dir/.akash0/cosmovisor/genesis/bin/akash" "$genesis_bin/akash"
			cp "$validators_dir/.akash0/cosmovisor/upgrades/$UPGRADE_TO/bin/akash" "$upgrade_bin/akash"
		fi

		AKASH=$genesis_bin/akash

		genesis_file=${valdir}/config/genesis.json
		rm -f "$genesis_file"

		$AKASH init --home "$valdir" "$(jq -rc '.moniker' <<<"$val")" >/dev/null 2>&1

		if [[ $cnt -eq 0 ]]; then
			cat >"$valdir/.envrc" <<EOL
PATH_add "\$(pwd)/cosmovisor/current/bin"
AKASH_HOME="\$(pwd)"
AKASH_FROM=validator0
AKASH_GAS=auto
AKASH_MINIMUM_GAS_PRICES=0.0025uakt
AKASH_NODE=tcp://127.0.0.1:26657
AKASH_CHAIN_ID=localakash
AKASH_KEYRING_BACKEND=test
AKASH_SIGN_MODE=direct

export AKASH_HOME
export AKASH_FROM
export AKASH_GAS
export AKASH_MINIMUM_GAS_PRICES
export AKASH_NODE
export AKASH_CHAIN_ID
export AKASH_KEYRING_BACKEND
export AKASH_SIGN_MODE
EOL
		fi

		jq -r '.keys.priv' <<<"$val" >"$valdir/config/priv_validator_key.json"
		jq -r '.keys.node' <<<"$val" >"$valdir/config/node_key.json"

		((cnt++)) || true
	done

	import_keys
	prepare_state
}

function prepare_state() {
	if [[ -z "${WORKDIR}" ]]; then
		echo "workdir is not set"
		echo -e "$USAGE"
		exit 1
	fi

	local config
	config=$(cat "$CONFIG_FILE")

	local cnt=0
	local validators_dir=${WORKDIR}/validators

	mkdir -p "${WORKDIR}/validators/logs"

	snapshot_file=${validators_dir}/snapshot

	for val in $(jq -c '.validators[]' <<<"$config"); do
		local valdir
		local cosmovisor_dir
		local genesis_bin
		local upgrade_bin
		local AKASH

		valdir=$validators_dir/.akash${cnt}
		cosmovisor_dir=$valdir/cosmovisor
		genesis_bin=$cosmovisor_dir/genesis/bin
		AKASH=$genesis_bin/akash

		genesis_file=${valdir}/config/genesis.json
		rm -f "$genesis_file"

		if [[ $cnt -eq 0 ]]; then
			if [[ "${GENESIS_URL}" =~ ^https?:\/\/.* ]]; then
				echo "Downloading genesis from ${GENESIS_URL}"

				pv_args="-petrafb -i 5"
				sz=$(content_size "${GENESIS_URL}")
				# shellcheck disable=SC2181
				if [ $? -eq 0 ]; then
					if [[ -n $sz ]]; then
						pv_args+=" -s $sz"
					fi

					tar_cmd=$(content_type "$(content_name "${GENESIS_URL}")")

					if [ "$tar_cmd" != "" ]; then
						# shellcheck disable=SC2086
						wget -nq -O - "${GENESIS_URL}" | pv $pv_args | eval "$tar_cmd"
					else
						wget -q --show-progress -O "$genesis_file" "${GENESIS_URL}"
					fi
				else
					echo "unable to download genesis"
				fi
			else
				echo "Unpacking genesis from ${GENESIS_URL}"
				tar_cmd=$(content_type "${GENESIS_URL}")
				# shellcheck disable=SC2086
				(pv -petrafb -i 5 "${GENESIS_URL}" | eval "$tar_cmd") 2>&1 | stdbuf -o0 tr '\r' '\n'
			fi

			if ! ls "${snapshot_file}"* 1> /dev/null 2>&1; then
				echo "Downloading snapshot to [$(pwd)] from $SNAPSHOT_URL..."
				file_url=$(content_location "$SNAPSHOT_URL")
				content_ext=$(content_type "$file_url")

				wget --show-progress -q -O "${snapshot_file}.${content_ext}" "$file_url"
			fi

			snap_file=${validators_dir}/$(find "$validators_dir" -name "snapshot.*" -type f -exec basename {} \;)

			tar_cmd=$(tar_by_content_type "$snap_file")

			pushd "$(pwd)"
			mkdir -p "${valdir}/data"
			cd "${valdir}/data"

			echo "Unpacking snapshot from $snap_file..."

			# shellcheck disable=SC2086
			(pv -petrafb -i 5 "$snap_file" | eval "$tar_cmd") 2>&1 | stdbuf -o0 tr '\r' '\n'

			# if snapshot provides data dir then move all things up
			if [[ -d data ]]; then
				echo "snapshot has data dir. moving content..."
				mv data/* ./
				rm -rf data
			fi

			popd

			$AKASH testnetify --home="$valdir" --testnet-rootdir="$validators_dir" --testnet-config="${STATE_CONFIG}" --yes || true

		else
			pushd "$(pwd)"
			cd "${valdir}"
			cp -r "${validators_dir}/.akash0/data" ./

			pushd "$(pwd)"

			cd "config"

			ln -snf "../../.akash0/config/genesis.json" "genesis.json"

			popd
			popd
		fi

		((cnt++)) || true
	done
}

function clean() {
	if [[ -z "${WORKDIR}" ]]; then
		echo "workdir is not set"
		echo -e "$USAGE"
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
		rm -rf "$cosmovisor_dir/upgrades/${UPGRADE_TO}/bin/akash"

		echo '{"height":"0","round": 0,"step": 0}' | jq > "$valdir/data/priv_validator_state.json"

		((cnt++)) || true
	done
}

function import_keys() {
	if [[ -z "${WORKDIR}" ]]; then
		echo "workdir is not set"
		echo -e "$USAGE"
		exit 1
	fi

	local config
	local validators_dir
	local cosmovisor_dir
	local genesis_bin
	local validators_dir

	config=$(cat "$CONFIG_FILE")

	validators_dir=${WORKDIR}/validators
	valdir=$validators_dir/.akash0
	cosmovisor_dir=$valdir/cosmovisor
	genesis_bin=$cosmovisor_dir/genesis/bin

	# upgrades may upgrade keys format so reset them as well
	rm -rf "$valdir"/keyring-test

	local AKASH
	AKASH=$genesis_bin/akash

	jq -c '.mnemonics[]' <<<"$config" | while read -r mnemonic; do
		jq -c '.keys[]' <<<"$mnemonic" | while read -r key; do
			jq -rc '.phrase' <<<"$mnemonic" | $AKASH --home="$valdir" --keyring-backend=test keys add "$(jq -rc '.name' <<<"$key")" --recover --index "$(jq -rc '.index' <<<"$key")"
		done
	done
}

function bins() {
	if [[ -z "${WORKDIR}" ]]; then
		echo "workdir is not set"
		echo -e "$USAGE"
		exit 1
	fi

	local config
	config=$(cat "$CONFIG_FILE")

	local cnt=0
	local validators_dir=${WORKDIR}/validators

	for val in $(jq -c '.validators[]' <<<"$config"); do
		local valdir
		local cosmovisor_dir
		local genesis_bin
		local upgrade_bin

		valdir=$validators_dir/.akash${cnt}
		cosmovisor_dir=$valdir/cosmovisor
		genesis_bin=$cosmovisor_dir/genesis/bin
		upgrade_bin=$cosmovisor_dir/upgrades/$UPGRADE_TO/bin

		mkdir -p "$genesis_bin"
		mkdir -p "$upgrade_bin"

		if [[ $cnt -eq 0 ]]; then
			build_bins "$genesis_bin" "$upgrade_bin"
		else
			cp "$validators_dir/.akash0/cosmovisor/genesis/bin/akash" "$genesis_bin/akash"
			cp "$validators_dir/.akash0/cosmovisor/upgrades/$UPGRADE_TO/bin/akash" "$upgrade_bin/akash"
		fi

		((cnt++)) || true
	done
}

case "$1" in
	init)
		shift
		init
		;;
	bins)
		shift
		bins
		;;
	keys)
		shift
		import_keys
		;;
	clean)
		shift
		clean
		;;
	prepare-state)
		shift
		prepare_state
		;;
	upgrade-from-release)
		shift
		upgrades_dir=${ROOT_DIR}/upgrades/software
		upgrade_name=$(find "${upgrades_dir}" -mindepth 1 -maxdepth 1 -type d | awk -F/ '{print $NF}' | sort -r | head -n 1)

		# shellcheck disable=SC2086
		res=$($semver validate $upgrade_name)
		if [[ "$res" == "valid" ]]; then
			echo -e "$upgrade_name"
			exit 0
		else
			exit 1
		fi

		;;
	test-required)
		shift
		curr_ref=$1

		upgrades_dir=${ROOT_DIR}/upgrades/software
		upgrade_name=$(find "${upgrades_dir}" -mindepth 1 -maxdepth 1 -type d | awk -F/ '{print $NF}' | sort -r | head -n 1)

		# shellcheck disable=SC2086
		is_valid=$($semver validate $upgrade_name)
		if [[ $is_valid != "valid" ]]; then
			echoerr "upgrade name \"$upgrade_name\" does not comply with semver spec"
			exit 1
		fi

		# current git reference is matching upgrade name. looks like release has been cut
		# so lets run the last test
		if [[ "$curr_ref" == "$upgrade_name" ]]; then
			echo -e "$upgrade_name"
			exit 0
		fi

		cnt=0

		retracted_versions=$(go mod edit --json | jq -cr .Retract)

		while :; do
			cnt=$((cnt + 1))
			if [[ $cnt -gt 100 ]]; then
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
		echo -e "$USAGE"
		exit 0
		;;
	*)
		echo "unknown command $1"
		echo -e "$USAGE"
		exit 1
		;;
esac
