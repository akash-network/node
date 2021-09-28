#!/bin/bash

#URL to get current USD price per AKT
DEFAULT_API_URL="https://api.coingecko.com/api/v3/simple/price?ids=akash-network&vs_currencies=usd"

# These are the variables one can modify to change the USD scale for each resource kind
CPU_USD_SCALE=0.10
MEMORY_USD_SCALE=0.02
STORAGE_USD_SCALE=0.01
ENDPOINT_USD_SCALE=0.02

# used later for validation
MAX_INT64=9223372036854775807

# local variables used for calculation
memory_total=0
cpu_total=0
storage_total=0
endpoint_total=0

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

  storage_quantity=$(jq '.storage' <<<"$group")
  storage_quantity=$((storage_quantity * count))
  storage_total=$((storage_total + storage_quantity))

  endpoint_quantity=$(jq '.endpoint_quantity' <<<"$group")
  endpoint_quantity=$((endpoint_quantity * count))
  endpoint_total=$((endpoint_total + endpoint_quantity))
done

# calculate the total cost in USD for each resource
cpu_cost_usd=$(bc -l <<<"${cpu_total}*${CPU_USD_SCALE}")
memory_cost_usd=$(bc -l <<<"${memory_total}*${MEMORY_USD_SCALE}")
storage_cost_usd=$(bc -l <<<"${storage_total}*${STORAGE_USD_SCALE}")
endpoint_cost_usd=$(bc -l <<<"${endpoint_total}*${ENDPOINT_USD_SCALE}")

# validate the USD cost for each resource
if [ 1 -eq "$(bc <<<"${cpu_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${cpu_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${memory_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${memory_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${storage_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${storage_cost_usd}<=${MAX_INT64}")" ] ||
  [ 1 -eq "$(bc <<<"${endpoint_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${endpoint_cost_usd}<=${MAX_INT64}")" ]; then
  exit 1
fi

# finally, calculate the total cost in USD of all resources and validate it
total_cost_usd=$(bc -l <<<"${cpu_cost_usd}+${memory_cost_usd}+${storage_cost_usd}+${endpoint_cost_usd}")
if [ 1 -eq "$(bc <<<"${total_cost_usd}<0")" ] || [ 0 -eq "$(bc <<<"${total_cost_usd}<=${MAX_INT64}")" ]; then
  exit 1
fi

# call the API and find out the current USD price per AKT
if [ -z "$API_URL" ]; then
  API_URL=$DEFAULT_API_URL
fi

API_RESPONSE=$(curl -s $API_URL)
curl_exit_status=$?
if [ $curl_exit_status != 0 ]; then
  exit $curl_exit_status
fi
usd_per_akt=$(jq '."akash-network"."usd"' <<<"$API_RESPONSE")

#validate the current USD price per AKT is not zero
if [ "$usd_per_akt" == 0 ]; then
  exit 1
fi

# calculate the total cost in uAKT
total_cost_akt=$(bc -l <<<"${total_cost_usd}/${usd_per_akt}")
total_cost_uakt=$(bc -l <<<"${total_cost_akt}*1000000")

# Round upwards to get an integer
total_cost_uakt=$(echo "$total_cost_uakt" | jq '.|ceil')

# return the price in uAKT
echo "$total_cost_uakt"
