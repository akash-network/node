#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SEMVER=$SCRIPT_DIR/semver.sh

gomod="$SCRIPT_DIR/../go.mod"

macos_deps=(
	"bash"
	"direnv"
	"pv"
	"lz4"
)

debian_deps=(
	"make"
	"build-essential"
	"direnv"
	"unzip"
	"wget"
	"curl"
	"npm"
	"jq"
	"coreutils"
)

is_command() {
	command -v "$1" >/dev/null
}

function get_gotoolchain() {
	local gotoolchain
	local goversion
	local local_goversion
	local toolfile

	toolfile=$gomod

	if [[ "$GOWORK" != "off" ]] && [ -f "$GOWORK" ]; then
		toolfile=$GOWORK
	fi

	gotoolchain=$(grep -E '^toolchain go[0-9]{1,}.[0-9]{1,}.[0-9]{1,}$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n')
	goversion=$(grep -E '^go [0-9]{1,}.[0-9]{1,}(.[0-9]{1,})?$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n')

	if [[ ${gotoolchain} == "" ]]; then
		gotoolchain=go$goversion
	fi

	if [[ ${gotoolchain} == "" ]]; then
		# determine go toolchain from go version in go.mod
		if which go >/dev/null 2>&1; then
			# shellcheck disable=SC2086
			local_goversion=$(env -i HOME="$HOME" GOTOOLCHAIN=local $SHELL -l -c "go version | cut -d ' ' -f 3 | sed 's/go*//' | tr -d '\n'")
			if ! [[ $($SEMVER compare "v$local_goversion" v"$goversion") -ge 0 ]]; then
				goversion=$local_goversion
			else
				local_goversion=
			fi
		fi

		if [[ "$local_goversion" == "" ]]; then
			goversion=$(curl -s "https://go.dev/dl/?mode=json&include=all" | jq -r --arg regexp "^go$goversion" '.[] | select(.stable == true) | select(.version | match($regexp)) | .version' | head -n 1 | sed -e s/^go//)
		fi

		if [[ $goversion != "" ]] && [[ $($SEMVER compare "v$goversion" v1.21.0) -ge 0 ]]; then
			gotoolchain=go${goversion}
		else
			gotoolchain=go$(grep -E '^go [0-9]{1,}.[0-9]{1,}$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n').0
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

	curr_module_name=$(go list -m)
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
	for retract in $(go mod edit --json | jq -cr '.Retract | if . != null then .[] else empty end'); do
		local low
		local high

		low=$(jq -r '.Low' <<<"$retract")
		high=$(jq -r '.High' <<<"$retract")
		echo "    dropping retract: [$low, $high]"
		go mod edit -dropretract=["$low","$high"]
	done

	replace_paths "./go.mod" "$curr_module_name" "$new_module_name"
}

function install_gha_deps() {
	if [[ "$OSTYPE" == "darwin"* ]]; then
		echo "Detected Darwin based system"

		if ! is_command brew; then
			echo "homebrew is not installed. visit https://brew.sh"
			exit 1
		fi

		local tools

		if ! is_command make || [[ $(make --version | head -1 | cut -d" " -f3 | cut -d"." -f1) -lt 4 ]]; then
			tools="$tools make"
		fi

		# shellcheck disable=SC2068
		for dep in ${macos_deps[@]}; do
			echo -n "detecting $dep ..."
			status="(installed)"
			if ! brew list "$dep" >/dev/null 2>&1; then
				tools="$tools $dep"
				status="(not installed)"
			fi

			echo " $status"
		done

		if [[ "$tools" != "" ]]; then
			# don't put quotes around $tools!
			# shellcheck disable=SC2086
			brew install $tools
		else
			echo "All requirements already met. Nothing to install"
		fi
	elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
		if is_command dpkg; then
			echo "Detected Debian based system"
			local tools

			# shellcheck disable=SC2068
			for dep in ${debian_deps[@]}; do
				echo -n "detecting $dep ..."
				status="(installed)"
				if ! dpkg -l "$dep" >/dev/null 2>&1; then
					tools="$tools $dep"
					status="(not installed)"
				fi
				echo " $status"
			done

			cmd="apt-get"

			if is_command sudo; then
				cmd="sudo $cmd"
			fi

			if [[ "$tools" != "" ]]; then
				$cmd update
				# don't put quotes around $tools!
				# shellcheck disable=SC2086
				(
					set -x
					$cmd install -y $tools
				)
			else
				echo "All requirements already met. Nothing to install"
			fi
		fi
	else
		echo "Unsupported OS $OSTYPE"
		exit 1
	fi
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
