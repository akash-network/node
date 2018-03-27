PROTO_FILES  = $(wildcard types/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))
PROGRAMS     = akash akashd

IMAGE_REPO ?= quay.io/ovrclk/akash
IMAGE_TAG  ?= latest

all: build $(PROGRAMS)

build:
	go build -i $$(glide novendor)

akash:
	go build ./cmd/akash

akashd:
	go build ./cmd/akashd

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
	goveralls -service=travis-ci -ignore="types/types.pb.go"

test-vet:
	go vet $$(glide novendor)

deps-install:
	glide install

devdeps-install:
	go get github.com/gogo/protobuf/protoc-gen-gogo
	go get github.com/vektra/mockery/.../

coverdeps-install:
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls

test-integration: $(PROGRAMS)
	(cd _integration && make clean run)

integrationdeps-install:
	(cd _integration && make deps-install)

gentypes: $(PROTOC_FILES)

%.pb.go: %.proto
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		--gogo_out=plugins=grpc:. $<

mocks:
	mockery -case=underscore -dir app/market -output app/market/mocks -name Client
	mockery -case=underscore -dir app/market -output app/market/mocks -name Engine
	mockery -case=underscore -dir app/market -output app/market/mocks -name Facilitator
	mockery -case=underscore -dir marketplace -output marketplace/mocks -name Handler

docs:
	(cd _docs/dot && make)

clean:
	rm -f $(PROGRAMS)

.PHONY: all build \
	akash akashd \
	image image-push \
	test test-full \
	deps-install devdeps-install \
	test-cover coverdeps-install \
	test-integraion integrationdeps-install \
	test-vet \
	mocks \
	docs \
	clean
