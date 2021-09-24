#!/bin/bash

MAX_INT=9223372036854775807
read input

CPU_SCALE=1
MEMORY_SCALE=2
STORAGE_SCALE=3
ENDPOINT_SCALE=4

HTTP_RESPONSE=$(curl "%{http_response}\n" "https://api.coingecko.com/api/v3/simple/price?ids=akash-network&vs_currencies=usd" --output output.txt --silent)
akt_price=$(jq '."akash-network"."usd"' <<< $HTTP_RESPONSE)

count=0

memory_quantity=0
memory_total=0

cpu_quantity=0
cpu_total=0

storage_quantity=0
storage_total=0

endpoint_quantity=0
endpoint_total=0

for i in $(jq -c '.[]' <<< $input)
do
  count=$(jq '.count' <<< $i)

     memory_quantity=$(jq '.memory' <<< $i)
     memory_quantity=$((memory_quantity*count))
     memory_total=$((memory_total+memory_quantity))

     cpu_quantity=$(jq '.cpu' <<< $i)
     cpu_quantity=$((cpu_quantity*count))
     cpu_total=$((cpu_total+cpu_quantity))

     storage_quantity=$(jq '.storage' <<< $i)
     storage_quantity=$((storage_quantity*count))
     storage_total=$((storage_total+storage_quantity))

     endpoint_quantity=$(jq '.endpoint_quantity' <<< $i)
     endpoint_quantity=$((endpoint_quantity*count))
     endpoint_total=$((endpoint_total+endpoint_quantity))
done

cpu_total=$((cpu_total*CPU_SCALE))
memory_total=$((memory_total*MEMORY_SCALE))
storage_total=$((storage_total*STORAGE_SCALE))
endpoint_total=$((endpoint_total*ENDPOINT_SCALE))

if [[ $cpu_total -lt 0  || $cpu_total -gt $MAX_INT
      || $memory_total -lt 0  || $memory_total -gt $MAX_INT
      || $storage_total -lt 0  || $storage_total -gt $MAX_INT
      || $endpoint_total -lt 0  || $endpoint_total -gt $MAX_INT ]]
then
  exit 1
fi

total_cost=$cpu_total
total_cost=$((total_cost+memory_total))
total_cost=$((total_cost+storage_total))
total_cost=$((total_cost+endpoint_total))

if [[ $total_cost -lt 0 || $total_cost -gt $MAX_INT ]]
then
  exit 1
fi

total_cost=$(echo $total_cost | jq '.|ceil')

total_cost=`echo $total_cost / $akt_price | bc`
total_cost=`echo $total_cost \* 1000000 | bc`

price=$(echo $total_cost | jq '.|ceil')
echo $price

