#!/bin/bash
i=0
while IFS= read -r line; do
	echo "${i} - ${line}"
    ((i=i+1))
done