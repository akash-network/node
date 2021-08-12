#!/usr/bin/env ash

FILE=/var/lib/testdata/test

if [[ -f $FILE ]]; then
	echo "test file exists. data survived"
	echo "content of the file"
	cat $FILE
else
	echo "initializing persistence test file"
	echo "Akash Persistence welcomes you" > $FILE
fi

/docker-entrypoint.sh
