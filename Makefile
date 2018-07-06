PROTO_FILES  = $(wildcard types/*.proto)
PROTOC_FILES = $(patsubst %.proto,%.pb.go, $(PROTO_FILES))

BINS       := akash akashd
IMAGE_BINS := _build/akash _build/akashd

IMAGE_BUILD_ENV = GOOS=linux GOARCH=amd64

all: build bins

bins: $(BINS)

build:
	go build -i $$(glide novendor)

akash:
	go build ./cmd/akash

akashd:
	go build ./cmd/akashd

image-bins:
	$(IMAGE_BUILD_ENV) go build -o _build/akash  ./cmd/akash
	$(IMAGE_BUILD_ENV) go build -o _build/akashd ./cmd/akashd

image: image-bins
	docker build --rm            \
		-t ovrclk/akash:latest     \
		-f _build/Dockerfile.akash \
		_build
	docker build --rm             \
		-t ovrclk/akashd:latest     \
		-f _build/Dockerfile.akashd \
		_build

install: akash akashd
	cp akash $(GOPATH)/bin
	cp akashd $(GOPATH)/bin

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

test-lint:
	golangci-lint run

lintdeps-install:
	go get -u github.com/golangci/golangci-lint/cmd/golangci-lint

test-vet:
	go vet $$(glide novendor | grep -v ./pkg/)

deps-install:
	glide install -v

devdeps-install:
	go get github.com/gogo/protobuf/protoc-gen-gogo
	go get github.com/vektra/mockery/.../
	go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway
	go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger

coverdeps-install:
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls

test-integration: $(BINS)
	(cd _integration && make clean run)

integrationdeps-install:
	(cd _integration && make deps-install)

gentypes: $(PROTOC_FILES)

kubetypes:
	vendor/k8s.io/code-generator/generate-groups.sh all \
  	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
  	akash.network:v1

%.pb.go: %.proto
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		-Ivendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		--gogo_out=plugins=grpc:. $<
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		-Ivendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		--grpc-gateway_out=logtostderr=true:. $<
	protoc -I. \
		-Ivendor -Ivendor/github.com/gogo/protobuf/protobuf \
		-Ivendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		--swagger_out=logtostderr=true:. $<

mocks:
	mockery -case=underscore -dir query                 -output query/mocks                 -name Client
	mockery -case=underscore -dir txutil                -output txutil/mocks                -name Client
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Client
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Engine
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Facilitator
	mockery -case=underscore -dir marketplace           -output marketplace/mocks           -name Handler
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Cluster
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Deployment
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Reservation
	mockery -case=underscore -dir provider/cluster/kube -output provider/cluster/kube/mocks -name Client
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name Handler


gofmt:
	find . -not -path './vendor*' -name '*.go' -type f | \
		xargs gofmt -s -w

docs:
	(cd _docs/dot && make)

clean:
	rm -f $(BINS) $(IMAGE_BINS)

.PHONY: all bins build \
	akash akashd \
	image image-bins \
	test test-nocache test-full \
	deps-install devdeps-install \
	test-cover coverdeps-install \
	test-integraion integrationdeps-install \
	test-lint lintdeps-install \
	test-vet \
	mocks \
	gofmt \
	docs \
	clean \
	kubetypes
