#!/usr/bin/env python3
import math
import sys
import json
import random

cpu_rate = 1
memory_rate = 2
storage_base_rate = 3 # For smaller deployments
storage_high_rate = 100 # For larger deployments
# The point at which a deployment becomes large in terms of storage
tier_threshold = 9.5 * (1024**3) # gigabytes

# Read JSON from standard input
order_data = json.load(sys.stdin)

# Store the total counts in these variables
total_cpu = 0
total_memory = 0
total_storage = 0

total_endpoints = 0

for group in order_data: # Iterate over the array
  # Add up what is being ordered
  total_cpu += group['cpu'] * group['count']
  total_memory += group['memory'] * group['count']
  total_storage += group['storage'] * group['count']
  total_endpoints += group['endpoint-quantity'] * group['count']

# Use the base rate by default
storage_rate = storage_base_rate

# If the deployment is large, then switch to the high rate
if total_storage >= tier_threshold:
  storage_rate = storage_high_rate

# Use units of megabytes
total_memory /= 1024**2
total_storage /= 1024**2

# Compute the final price that is used for the bid
price = cpu_rate * total_cpu
price += memory_rate * total_memory
price += storage_rate * total_storage

if total_endpoints > 2:
  price += random.randint(50000, 300000)

price += random.randint(-2, 2)

price = max(price, 0)

# Round upwards, then convert to an integer. Write to standard out as JSON
json.dump(int(math.ceil(price)), sys.stdout)


