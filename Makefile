PROTO_FILES  = $(wildcard types/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))
PROGRAMS     = photon photond

IMAGE_REPO ?= quay.io/ovrclk/photon
IMAGE_TAG  ?= latest

all: build $(PROGRAMS)

build:
	go build -i $$(glide novendor)

photon:
	go build ./cmd/photon

photond:
	go build ./cmd/photond

image:
	docker build --rm -t $(IMAGE_REPO):$(IMAGE_TAG) .

image-push:
	docker push $(IMAGE_REPO):$(IMAGE_TAG)

image-minikube:
	eval $$(minikube docker-env) && make image

test:
	go test $$(glide novendor)

test-full:
	go test -race $$(glide novendor)

test-cover:
	goveralls -service=travis-pro -ignore=types.pb.go

test-vet:
	go vet $$(glide novendor)

deps-install:
	glide install

devdeps-install:
	go get github.com/gogo/protobuf/protoc-gen-gogo
	go get -u github.com/cloudflare/cfssl/cmd/...

coverdeps-install:
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls

gentypes: $(PROTOC_FILES)

%.pb.go: %.proto
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		--gogo_out=plugins=grpc:. $<

docs:
	(cd _docs/dot && make)

clean:
	rm -f $(PROGRAMS)

.PHONY: all build \
	photon photond \
	image image-push \
	test test-full \
	deps-install devdeps-install \
	test-cover coverdeps-install \
	test-vet \
	docs \
	clean
