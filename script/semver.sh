#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o nounset -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

source "$SCRIPT_DIR/semver_funcs.sh"

PROG=semver
PROG_VERSION="3.4.0"

USAGE="\
Usage:
  $PROG bump major <version>
  $PROG bump minor <version>
  $PROG bump patch <version>
  $PROG bump prerel|prerelease [<prerel>] <version>
  $PROG bump build <build> <version>
  $PROG bump release <version>
  $PROG get major <version>
  $PROG get minor <version>
  $PROG get patch <version>
  $PROG get prerel|prerelease <version>
  $PROG get build <version>
  $PROG get release <version>
  $PROG compare <version> <other_version>
  $PROG diff <version> <other_version>
  $PROG validate <version>
  $PROG --help
  $PROG --version

Arguments:
  <version>  A version must match the following regular expression:
             \"${SEMVER_REGEX}\"
             In English:
             -- The version must match X.Y.Z[-PRERELEASE][+BUILD]
                where X, Y and Z are non-negative integers.
             -- PRERELEASE is a dot separated sequence of non-negative integers and/or
                identifiers composed of alphanumeric characters and hyphens (with
                at least one non-digit). Numeric identifiers must not have leading
                zeros. A hyphen (\"-\") introduces this optional part.
             -- BUILD is a dot separated sequence of identifiers composed of alphanumeric
                characters and hyphens. A plus (\"+\") introduces this optional part.

  <other_version>  See <version> definition.

  <prerel>  A string as defined by PRERELEASE above. Or, it can be a PRERELEASE
            prototype string followed by a dot.

  <build>   A string as defined by BUILD above.

Options:
  -v, --version          Print the version of this tool.
  -h, --help             Print this help message.

Commands:
  bump      Bump by one of major, minor, patch; zeroing or removing
            subsequent parts. \"bump prerel\" (or its synonym \"bump prerelease\")
            sets the PRERELEASE part and removes any BUILD part. A trailing dot
            in the <prerel> argument introduces an incrementing numeric field
            which is added or bumped. If no <prerel> argument is provided, an
            incrementing numeric field is introduced/bumped. \"bump build\" sets
            the BUILD part.  \"bump release\" removes any PRERELEASE or BUILD parts.
            The bumped version is written to stdout.

  get       Extract given part of <version>, where part is one of major, minor,
            patch, prerel (alternatively: prerelease), build, or release.

  compare   Compare <version> with <other_version>, output to stdout the
            following values: -1 if <other_version> is newer, 0 if equal, 1 if
            older. The BUILD part is not used in comparisons.

  diff      Compare <version> with <other_version>, output to stdout the
            difference between two versions by the release type (MAJOR, MINOR,
            PATCH, PRERELEASE, BUILD).

  validate  Validate if <version> follows the SEMVER pattern (see <version>
            definition). Print 'valid' to stdout if the version is valid, otherwise
            print 'invalid'.

See also:
  https://semver.org -- Semantic Versioning 2.0.0"

function usage_help {
	error "$USAGE"
}

function usage_version {
	echo -e "${PROG}: $PROG_VERSION"
	exit 0
}

case $# in
	0)
		echo "Unknown command: $*"
		usage_help
		;;
esac

case $1 in
	--help | -h)
		echo -e "$USAGE"
		exit 0
		;;
	--version | -v) usage_version ;;
	bump)
		shift
		command_bump "$@"
		;;
	get)
		shift
		command_get "$@"
		;;
	compare)
		shift
		command_compare "$@"
		;;
	diff)
		shift
		command_diff "$@"
		;;
	validate)
		shift
		command_validate "$@"
		;;
	*)
		echo "Unknown arguments: $*"
		usage_help
		;;
esac
