
PROTO_FILES  = $(wildcard api/*/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))

build:
	go build -i $$(glide novendor)

test:
	go test $$(glide novendor)

test-full:
	go test -race $$(glide novendor)

deps-install:
	glide install

devdeps-install:
	go get -u github.com/golang/protobuf/protoc-gen-go
	go get -u github.com/cloudflare/cfssl/cmd/...

genapi: $(PROTOC_FILES)

%.pb.go: %.proto
	protoc --go_out=plugins=grpc:. $<

.PHONY: build test test-full \
	deps-install devdeps-install
