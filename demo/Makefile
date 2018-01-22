build :
				go build ./cmd/client
				go build ./cmd/node

client:
				rm -f client
				go build ./cmd/client

node:
				rm -f node
				go build ./cmd/node

clean:
				rm -f client
				rm -f node
				rm -rf ${HOME}/.demonode
				rm -rf ${HOME}/.democlient
