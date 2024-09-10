#!/usr/bin/env bash

NAT='0|[1-9][0-9]*'
ALPHANUM='[0-9]*[A-Za-z-][0-9A-Za-z-]*'
IDENT="$NAT|$ALPHANUM"
FIELD='[0-9A-Za-z-]+'

SEMVER_REGEX_STR="\
[vV]?\
($NAT)\\.($NAT)\\.($NAT)\
(\\-(${IDENT})(\\.(${IDENT}))*)?\
(\\+${FIELD}(\\.${FIELD})*)?$"

SEMVER_REGEX_LEGACY="\
[vV]?\
($NAT)\\.($NAT)(\\.($NAT))?\
(\\-(${IDENT})(\\.(${IDENT}))*)?\
(\\+${FIELD}(\\.${FIELD})*)?$"

SEMVER_REGEX="^$SEMVER_REGEX_STR"

function error {
	echo -e "$1" >&2
	exit 1
}

# normalize the "part" keywords to a canonical string.  At present,
# only "prerelease" is normalized to "prerel".

function normalize_part {
	if [ "$1" == "prerelease" ]; then
		echo "prerel"
	else
		echo "$1"
	fi
}

function validate_version {
	local version=$1
	if [[ "$version" =~ $SEMVER_REGEX ]]; then
		# if a second argument is passed, store the result in var named by $2
		if [ "$#" -eq "2" ]; then
			local major=${BASH_REMATCH[1]}
			local minor=${BASH_REMATCH[2]}
			local patch=${BASH_REMATCH[3]}
			local prere=${BASH_REMATCH[4]}
			local build=${BASH_REMATCH[8]}
			eval "$2=(\"$major\" \"$minor\" \"$patch\" \"$prere\" \"$build\")"
		else
			echo "$version"
		fi
	elif [[ "$version" =~ $SEMVER_REGEX_LEGACY ]]; then
		# if a second argument is passed, store the result in var named by $2
		if [[ "$#" -eq "2" ]]; then
			local major=${BASH_REMATCH[1]}
			local minor=${BASH_REMATCH[2]}
			local patch=0
			local prere=${BASH_REMATCH[4]}
			local build=${BASH_REMATCH[6]}
			eval "$2=(\"${major}\" \"${minor}\" \"${patch}\" \"${prere}\" \"${build}\")"
		else
			echo "$version"
		fi
	else
		error "version $version does not match the semver scheme 'X.Y.Z(-PRERELEASE)(+BUILD)'. See help for more information."
	fi
}

function is_nat {
	[[ "$1" =~ ^($NAT)$ ]]
}

function is_null {
	[ -z "$1" ]
}

function order_nat {
	[ "$1" -lt "$2" ] && {
		echo -1
		return
	}
	[ "$1" -gt "$2" ] && {
		echo 1
		return
	}
	echo 0
}

function order_string {
	[[ $1 < $2 ]] && {
		echo -1
		return
	}
	[[ $1 > $2 ]] && {
		echo 1
		return
	}
	echo 0
}

# given two (named) arrays containing NAT and/or ALPHANUM fields, compare them
# one by one according to semver 2.0.0 spec. Return -1, 0, 1 if left array ($1)
# is less-than, equal, or greater-than the right array ($2).  The longer array
# is considered greater-than the shorter if the shorter is a prefix of the longer.
#
function compare_fields {
	local l="$1[@]"
	local r="$2[@]"
	local leftfield=("${!l}")
	local rightfield=("${!r}")
	local left
	local right

	local i=$((-1))
	local order=$((0))

	while true; do
		# shellcheck disable=SC2086
		[ $order -ne 0 ] && {
			echo "$order"
			return
		}

		: $((i++))
		left="${leftfield[$i]}"
		right="${rightfield[$i]}"

		is_null "$left" && is_null "$right" && {
			echo 0
			return
		}
		is_null "$left" && {
			echo -1
			return
		}
		is_null "$right" && {
			echo 1
			return
		}

		is_nat "$left" && is_nat "$right" && {
			order=$(order_nat "$left" "$right")
			continue
		}
		is_nat "$left" && {
			echo -1
			return
		}
		is_nat "$right" && {
			echo 1
			return
		}
		{
			order=$(order_string "$left" "$right")
			continue
		}
	done
}

# shellcheck disable=SC2206     # checked by "validate"; ok to expand prerel id's into array
function compare_version {
	local order
	validate_version "$1" V
	validate_version "$2" V_

	# compare major, minor, patch

	local left=("${V[0]}" "${V[1]}" "${V[2]}")
	local right=("${V_[0]}" "${V_[1]}" "${V_[2]}")

	order=$(compare_fields left right)
	[ "$order" -ne 0 ] && {
		echo "$order"
		return
	}

	# compare pre-release ids when M.m.p are equal

	local prerel="${V[3]:1}"
	local prerel_="${V_[3]:1}"
	local left=(${prerel//./ })
	local right=(${prerel_//./ })

	# if left and right have no pre-release part, then left equals right
	# if only one of left/right has pre-release part, that one is less than simple M.m.p

	[ -z "$prerel" ] && [ -z "$prerel_" ] && {
		echo 0
		return
	}
	[ -z "$prerel" ] && {
		echo 1
		return
	}
	[ -z "$prerel_" ] && {
		echo -1
		return
	}

	# otherwise, compare the pre-release id's

	compare_fields left right
}

# render_prerel -- return a prerel field with a trailing numeric string
#                  usage: render_prerel numeric [prefix-string]
#
function render_prerel {
	if [ -z "$2" ]; then
		echo "${1}"
	else
		echo "${2}${1}"
	fi
}

# extract_prerel -- extract prefix and trailing numeric portions of a pre-release part
#                   usage: extract_prerel prerel prerel_parts
#                   The prefix and trailing numeric parts are returned in "prerel_parts".
#
PREFIX_ALPHANUM='[.0-9A-Za-z-]*[.A-Za-z-]'
DIGITS='[0-9][0-9]*'
EXTRACT_REGEX="^(${PREFIX_ALPHANUM})*(${DIGITS})$"

function extract_prerel {
	local prefix
	local numeric

	if [[ "$1" =~ $EXTRACT_REGEX ]]; then # found prefix and trailing numeric parts
		prefix="${BASH_REMATCH[1]}"
		numeric="${BASH_REMATCH[2]}"
	else # no numeric part
		prefix="${1}"
		numeric=
	fi

	eval "$2=(\"$prefix\" \"$numeric\")"
}

# bump_prerel -- return the new pre-release part based on previous pre-release part
#                and prototype for bump
#                usage: bump_prerel proto previous
#
function bump_prerel {
	local proto
	local prev_prefix
	local prev_numeric

	# case one: no trailing dot in prototype => simply replace previous with proto
	if [[ ! ("$1" =~ \.$) ]]; then
		echo "$1"
		return
	fi

	proto="${1%.}" # discard trailing dot marker from prototype

	extract_prerel "${2#-}" prerel_parts # extract parts of previous pre-release
	#   shellcheck disable=SC2154
	prev_prefix="${prerel_parts[0]}"
	prev_numeric="${prerel_parts[1]}"

	# case two: bump or append numeric to previous pre-release part
	if [ "$proto" == "+" ]; then # dummy "+" indicates no prototype argument provided
		if [ -n "$prev_numeric" ]; then
			: $((++prev_numeric)) # previous pre-release is already numbered, bump it
			render_prerel "$prev_numeric" "$prev_prefix"
		else
			render_prerel 1 "$prev_prefix" # append starting number
		fi
		return
	fi

	# case three: set, bump, or append using prototype prefix
	if [ "$prev_prefix" != "$proto" ]; then
		render_prerel 1 "$proto" # proto not same pre-release; set and start at '1'
	elif [ -n "$prev_numeric" ]; then
		: $((++prev_numeric)) # pre-release is numbered; bump it
		render_prerel "$prev_numeric" "$prev_prefix"
	else
		render_prerel 1 "$prev_prefix" # start pre-release at number '1'
	fi
}

function command_bump {
	local new
	local version
	local sub_version
	local command

	command="$(normalize_part "$1")"

	case $# in
		2) case "$command" in
			major | minor | patch | prerel | release)
				sub_version="+."
				version=$2
				;;
			*) usage_help ;;
		esac ;;
		3) case "$command" in
			prerel | build) sub_version=$2 version=$3 ;;
			*) usage_help ;;
		esac ;;
		*) usage_help ;;
	esac

	validate_version "$version" parts
	# shellcheck disable=SC2154
	local major="${parts[0]}"
	local minor="${parts[1]}"
	local patch="${parts[2]}"
	local prere="${parts[3]}"
	local build="${parts[4]}"

	case "$command" in
		major) new="$((major + 1)).0.0" ;;
		minor) new="${major}.$((minor + 1)).0" ;;
		patch) new="${major}.${minor}.$((patch + 1))" ;;
		release) new="${major}.${minor}.${patch}" ;;
		prerel) new=$(validate_version "${major}.${minor}.${patch}-$(bump_prerel "$sub_version" "$prere")") ;;
		build) new=$(validate_version "${major}.${minor}.${patch}${prere}+${sub_version}") ;;
		*) usage_help ;;
	esac

	echo "$new"
	exit 0
}

function command_compare {
	local v
	local v_

	case $# in
		2)
			v=$(validate_version "$1")
			v_=$(validate_version "$2")
			;;
		*) usage_help ;;
	esac

	set +u # need unset array element to evaluate to null
	compare_version "$v" "$v_"
	exit 0
}

function command_diff {
	validate_version "$1" v1_parts
	# shellcheck disable=SC2154
	local v1_major="${v1_parts[0]}"
	local v1_minor="${v1_parts[1]}"
	local v1_patch="${v1_parts[2]}"
	local v1_prere="${v1_parts[3]}"
	local v1_build="${v1_parts[4]}"

	validate_version "$2" v2_parts
	# shellcheck disable=SC2154
	local v2_major="${v2_parts[0]}"
	local v2_minor="${v2_parts[1]}"
	local v2_patch="${v2_parts[2]}"
	local v2_prere="${v2_parts[3]}"
	local v2_build="${v2_parts[4]}"

	if [ "${v1_major}" != "${v2_major}" ]; then
		echo "major"
	elif [ "${v1_minor}" != "${v2_minor}" ]; then
		echo "minor"
	elif [ "${v1_patch}" != "${v2_patch}" ]; then
		echo "patch"
	elif [ "${v1_prere}" != "${v2_prere}" ]; then
		echo "prerelease"
	elif [ "${v1_build}" != "${v2_build}" ]; then
		echo "build"
	fi
}

# shellcheck disable=SC2034
function command_get {
	local part version

	if [[ "$#" -ne "2" ]] || [[ -z "$1" ]] || [[ -z "$2" ]]; then
		usage_help
		exit 0
	fi

	part="$1"
	version="$2"

	validate_version "$version" parts
	local major="${parts[0]}"
	local minor="${parts[1]}"
	local patch="${parts[2]}"
	local prerel="${parts[3]:1}"
	local build="${parts[4]:1}"
	local release="${major}.${minor}.${patch}"

	part="$(normalize_part "$part")"

	case "$part" in
		major | minor | patch | release | prerel | build) echo "${!part}" ;;
		*) usage_help ;;
	esac

	exit 0
}

function command_validate {
	if [[ "$#" -ne "1" ]]; then
		usage_help
	fi

	if [[ "$1" =~ $SEMVER_REGEX ]]; then
		echo "valid"
	else
		echo "invalid"
	fi

	exit 0
}
