build:
				go build ./cmd/client
				go build ./cmd/node

dockerbuild:
				env GOOS=linux GOARCH=amd64 go build ./cmd/client
				env GOOS=linux GOARCH=amd64 go build ./cmd/node

client:
				go build ./cmd/client

node:
				go build ./cmd/node

clean:
				rm -f ./client
				rm -f ./node
				rm -rf ./data

.PHONY: client node build clean dockerbuild
