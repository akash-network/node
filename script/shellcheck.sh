#!/bin/bash

# Ignore file
IGNORE="godownloader.sh"

unset FAILED

FILES=$(find /shellcheck/ -type f -name "*.sh" ! -path "/shellcheck/vendor/*" ! -path "/shellcheck/.git/*")

for file in $FILES; do
    name="$(basename "$file")";
    if [[ $name != "$IGNORE"  ]] && ! shellcheck --format=gcc "${file}" --exclude=SC1091; then
        export FAILED=true
    fi
done
if [ "${FAILED}" != "" ]; then exit 1; fi
