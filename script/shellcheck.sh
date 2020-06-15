#!/bin/bash

unset FAILED

FILES=$(find /shellcheck/ -type f -name "*.sh")

for file in $FILES; do
    if ! shellcheck --format=gcc "${file}" --exclude=SC1091; then
        export FAILED=true
    fi
done
if [ "${FAILED}" != "" ]; then exit 1; fi
