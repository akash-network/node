
PROTO_FILES  = $(wildcard types/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))
PROGRAMS     = photon photond

all: build $(PROGRAMS)

build:
	go build -i $$(glide novendor)

build-linux:
	env GOOS=linux GOARCH=amd64 go build -o photon-linux -i ./cmd/photon
	env GOOS=linux GOARCH=amd64 go build -o photond-linux -i ./cmd/photond

photon:
	go build ./cmd/photon

photond:
	go build ./cmd/photond

image:
	docker build --rm -t quay.io/ovrclk/photon:demo .

push-image:
	docker push quay.io/ovrclk/photon:demo

configmap:
	@$(MAKE) -C _demo

test:
	go test $$(glide novendor)

test-full:
	go test -race $$(glide novendor)

deps-install:
	glide install

devdeps-install:
	go get github.com/gogo/protobuf/protoc-gen-gogo
	go get -u github.com/cloudflare/cfssl/cmd/...

gentypes: $(PROTOC_FILES)

%.pb.go: %.proto
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		--gogo_out=plugins=grpc:. $<

docs:
	(cd _docs/dot && make)

clean:
	rm -f $(PROGRAMS)

.PHONY: all build build-linux \
	photon photond \
	image push-image \
	configmap \
	test test-full \
	deps-install devdeps-install \
	docs \
	clean
