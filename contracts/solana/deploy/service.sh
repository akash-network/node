#!/usr/bin/env bash
# Production service entry point. No implicit local stack or signing mode.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
if (($# != 2)) || [[ "$2" != /* ]]; then
  echo 'usage: service.sh relayer|reporter-preview|reporter-broadcast /absolute/config.json' >&2
  exit 2
fi
case "$1" in
  relayer) entry=local/relay.ts; mode=() ;;
  reporter-preview) entry=reporter/run.ts; mode=() ;;
  reporter-broadcast) entry=reporter/run.ts; mode=(--broadcast) ;;
  *) echo "unknown service: $1" >&2; exit 2 ;;
esac
cd "$repo"
node --import tsx deploy/service-config.ts "$1" "$2"
exec node --import tsx "$entry" --config "$2" --watch "${mode[@]}"
