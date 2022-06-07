#!/bin/bash

# checking memory leak
for ((i = 1; i <= 10000; i++)); do
    echo "--- Iteration #$i: $(date) ---"
    make start
    make stop
    make delete
    sleep 1
done