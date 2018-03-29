PROTO_FILES  = $(wildcard types/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))

BINS       := akash akashd
IMAGE_BINS := akash-docker akashd-docker

IMAGE_REPO ?= quay.io/ovrclk/akash
IMAGE_TAG  ?= latest

IMAGE_BUILD_ENV = GOOS=linux GOARCH=amd64
ifdef TRAVIS
	IMAGE_LDFLAGS += -X github.com/ovrclk/akash/version.version="$(TRAVIS_BRANCH)" \
									 -X github.com/ovrclk/akash/version.commit="$(TRAVIS_COMMIT)"
endif

all: build $(BINS)

build:
	go build -i $$(glide novendor)

akash:
	go build ./cmd/akash

akashd:
	go build ./cmd/akashd

image-bins:
	$(IMAGE_BUILD_ENV) go build -ldflags '$(IMAGE_LDFLAGS)' -o akash-docker  ./cmd/akash
	$(IMAGE_BUILD_ENV) go build -ldflags '$(IMAGE_LDFLAGS)' -o akashd-docker ./cmd/akashd

image: image-bins
	docker build --rm -t $(IMAGE_REPO):$(IMAGE_TAG) .

image-minikube:
	eval $$(minikube docker-env) && make image

test:
	go test $$(glide novendor)

test-nocache:
	go test -count=1 $$(glide novendor)

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

test-integration: $(BINS)
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
	rm -f $(BINS) $(IMAGE_BINS)

.PHONY: all build \
	akash akashd \
	image image-bins \
	test test-nocache test-full \
	deps-install devdeps-install \
	test-cover coverdeps-install \
	test-integraion integrationdeps-install \
	test-vet \
	mocks \
	docs \
	clean
