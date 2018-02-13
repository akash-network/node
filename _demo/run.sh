#!/bin/sh
mkdir -p data/$1
cp static_data/$1/* data/$1/
./photond-linux start --home=data/$1

