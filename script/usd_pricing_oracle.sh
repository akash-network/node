#!/usr/bin/env bash
# shellcheck shell=bash

set -e

if [[ "$SHELL" == "bash" ]]; then
  if [ "${BASH_VERSINFO:-0}" -lt 4 ]; then
    echo "the script needs BASH 4 or above" >&2
    exit 1
  fi
fi

#  To run this script, the following commands need to be installed:
#
# * jq 1.5.1 or newer
# * bc
# * curl

if ! command -v jq &> /dev/null ; then
  echo "jq could not be found"
  exit 1
fi

if ! command -v bc &> /dev/null ; then
  echo "bc could not be found"
  exit 1
fi

if ! command -v curl &> /dev/null ; then
  echo "curl could not be found"
  exit 1
fi

# One can set API_URL env variable to the url that returns coingecko like response, something like: `{"akash-network":{"usd":3.57}}`
# and if the API_URL isn't set, the default api url will be used

# URL to get current USD price per AKT
DEFAULT_API_URL="https://api.coingecko.com/api/v3/simple/price?ids=akash-network&vs_currencies=usd"

# These are the variables one can modify to change the USD scale for each resource kind
CPU_USD_SCALE=0.10
MEMORY_USD_SCALE=0.02
ENDPOINT_USD_SCALE=0.02

declare -A STORAGE_USD_SCALE

STORAGE_USD_SCALE[ephemeral]=0.01
STORAGE_USD_SCALE[default]=0.02
STORAGE_USD_SCALE[beta1]=0.02
STORAGE_USD_SCALE[beta2]=0.03
STORAGE_USD_SCALE[beta3]=0.04

# used later for validation
MAX_INT64=9223372036854775807

# local variables used for calculation
memory_total=0
cpu_total=0
endpoint_total=0
storage_cost_usd=0

# read the JSON in `stdin` into $script_input
read -r script_input

# iterate over all the groups and calculate total quantity of each resource
for group in $(jq -c '.[]' <<<"$script_input"); do
  count=$(jq '.count' <<<"$group")

  memory_quantity=$(jq '.memory' <<<"$group")
  memory_quantity=$((memory_quantity * count))
  memory_total=$((memory_total + memory_quantity))

  cpu_quantity=$(jq '.cpu' <<<"$group")
  cpu_quantity=$((cpu_quantity * count))
  cpu_total=$((cpu_total + cpu_quantity))

  for storage in $(jq -c '.storage[]' <<<"$group"); do
      storage_size=$(jq -r '.size' <<<"$storage")
      # jq has to be with -r to not quote value
      class=$(jq -r '.class' <<<"$storage")

      if [ -v 'STORAGE_USD_SCALE[class]' ]; then
        echo "requests unsupported storage class \"$class\"" >&2
        exit 1
      fi

      storage_size=$((storage_size * count))
      storage_cost_usd=$(bc -l <<<"(${storage_size}*${STORAGE_USD_SCALE[$class]}) + ${storage_cost_usd}")
  done

  endpoint_quantity=$(jq ".endpoint_quantity" <<<"$group")
  endpoint_quantity=$((endpoint_quantity * count))
  endpoint_total=$((endpoint_total + endpoint_quantity))
done

# calculate the total cost in USD for each resource
cpu_cost_usd=$(bc -l <<<"${cpu_total}*${CPU_USD_SCALE}")
memory_cost_usd=$(bc -l <<<"${memory_total}*${MEMORY_USD_SCALE}")
endpoint_cost_usd=$(bc -l <<<"${endpoint_total}*${ENDPOINT_USD_SCALE}")

# validate the USD cost for each resource
if [ 1 -eq "$(bc <<<"${cpu_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${cpu_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${memory_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${memory_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${storage_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${storage_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${endpoint_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${endpoint_cost_usd}<=${MAX_INT64}")" ]; then
  echo "invalid cost results for units" >&2
  exit 1
fi

# finally, calculate the total cost in USD of all resources and validate it
total_cost_usd=$(bc -l <<<"${cpu_cost_usd}+${memory_cost_usd}+${storage_cost_usd}+${endpoint_cost_usd}")
if [ 1 -eq "$(bc <<<"${total_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${total_cost_usd}<=${MAX_INT64}")" ]; then
  echo "invalid total cost $total_cost_usd" >&2
  exit 1
fi

# call the API and find out the current USD price per AKT
if [ -z "$API_URL" ]; then
  API_URL=$DEFAULT_API_URL
fi

API_RESPONSE=$(curl -s "$API_URL")
curl_exit_status=$?
if [ $curl_exit_status != 0 ]; then
  exit $curl_exit_status
fi
usd_per_akt=$(jq '."akash-network"."usd"' <<<"$API_RESPONSE")

# validate the current USD price per AKT is not zero
if [ 1 -eq "$(bc <<< "${usd_per_akt}==0")" ]; then
  exit 1
fi

# calculate the total cost in uAKT
total_cost_akt=$(bc -l <<<"${total_cost_usd}/${usd_per_akt}")
total_cost_uakt=$(bc -l <<<"${total_cost_akt}*1000000")

# Round upwards to get an integer
total_cost_uakt=$(echo "$total_cost_uakt" | jq 'def ceil: if . | floor == . then . else . + 1.0 | floor end; .|ceil')

# return the price in uAKT
echo "$total_cost_uakt"
