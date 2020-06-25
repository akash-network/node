#!/bin/sh

set -e

##
# Configuration sanity check
##

# shellcheck disable=SC2015
[ -f "$AKASHCTL_BOOT_KEYS/key.txt" ] && [ -f "$AKASHCTL_BOOT_KEYS/key-pass.txt" ] || {
  echo "Key information not found; AKASHCTL_BOOT_KEYS is not configured properly"
  exit 1
}

##
# Import key
##
/akashctl keys import "$AKASHCTL_FROM" \
  "$AKASHCTL_BOOT_KEYS/key.txt" < "$AKASHCTL_BOOT_KEYS/key-pass.txt"

##
# Run daemon
##
/akashctl provider run --cluster-k8s
