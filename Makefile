BINS       := akash
IMAGE_BINS := _build/akash
APP_DIR := ./app

GO := GO111MODULE=on go
GOBIN := $(shell go env GOPATH)/bin

KIND_APP_IP ?= $(shell make -sC _run/kube kind-k8s-ip)
KIND_APP_PORT ?= $(shell make -sC _run/kube app-http-port)
KIND_VARS ?= KIND_APP_IP="$(KIND_APP_IP)" KIND_APP_PORT="$(KIND_APP_PORT)"

# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifeq ($(MAINNET),true)
	BUILD_MAINNET=mainnet
endif

GOLANGCI_LINT_VERSION = v1.27.0

IMAGE_BUILD_ENV = GOOS=linux GOARCH=amd64

BUILD_FLAGS = -mod=readonly -tags "netgo ledger $(BUILD_MAINNET)" -ldflags \
 '-X github.com/cosmos/cosmos-sdk/version.Name=akash \
  -X github.com/cosmos/cosmos-sdk/version.AppName=akash \
  -X "github.com/cosmos/cosmos-sdk/version.BuildTags=netgo,ledger" \
  -X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
  -X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')'

all: build bins

bins: $(BINS)

build:
	$(GO) build ./...

generate:
	$(GO) generate ./...

akash:
	$(GO) build $(BUILD_FLAGS) ./cmd/akash

image-bins:
	$(IMAGE_BUILD_ENV) $(GO) build $(BUILD_FLAGS) -o _build/akash ./cmd/akash

image: image-bins
	docker build --rm             \
		-t ovrclk/akash:latest     \
		-f _build/Dockerfile.akash \
		_build

install:
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

release:
	docker run --rm --privileged \
	-v $(PWD):/go/src/github.com/ovrclk/akash \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-w /go/src/github.com/ovrclk/akash \
	-e GITHUB_TOKEN \
	-e DOCKER_USERNAME \
	-e DOCKER_PASSWORD \
	-e DOCKER_REGISTRY \
	goreleaser/goreleaser release --rm-dist

image-minikube:
	eval $$(minikube docker-env) && make image

shellcheck:
	docker run --rm \
	--volume ${PWD}:/shellcheck \
	--entrypoint sh \
	koalaman/shellcheck-alpine:stable \
	-x /shellcheck/script/shellcheck.sh

test:
	$(GO) test -tags=$(BUILD_MAINNET)  -timeout 300s ./...

test-nocache:
	$(GO) test -tags=$(BUILD_MAINNET) -count=1 ./...

test-full:
	$(GO) test -tags=$(BUILD_MAINNET) -race ./...

test-coverage:
	$(GO) test -tags=$(BUILD_MAINNET) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="./..." \
		./...

test-lint:
	golangci-lint run

lintdeps-install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

SUBLINTERS = deadcode \
			misspell \
			goerr113 \
			gofmt \
			gocritic \
			goconst \
			ineffassign \
			unparam \
			staticcheck \
			golint \
			gosec \
			scopelint \
			prealloc	

# TODO: ^ gochecknoglobals

LINT = $(GOBIN)/golangci-lint run ./... --disable-all --deadline=5m --enable 

# Execute the same lint methods as configured in .github/workflows/tests.yaml
# Clear feedback from each method as it fails.
test-sublinters: $(patsubst %, test-sublinter-%,$(SUBLINTERS))

test-sublinter-misspell:
	$(LINT) misspell --no-config

test-sublinter-ineffassign:
	$(LINT) ineffassign --no-config

test-sublinter-%:
	$(LINT) "$(@:test-sublinter-%=%)"

test-vet:
	$(GO) vet ./...

deps-install:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

devdeps-install: lintdeps-install kubetypes-deps-install
	$(GO) install github.com/vektra/mockery/.../
	$(GO) install k8s.io/code-generator/...
	$(GO) install sigs.k8s.io/kind
	$(GO) install golang.org/x/tools/cmd/stringer

test-integration: $(BINS)
	cp akash ./_build
	go test -mod=readonly -p 4 -tags "integration $(BUILD_MAINNET)" -v ./integration/...

test-e2e-integration: $(BINS)
	# ASSUMES:
	# 1. cluster created - `kind create cluster --config=_run/kube/kind-config.yaml`
	# 2. cluster setup - `make -s -C _run/kube kind-ingress-setup`
	cp akashctl akashd ./_build
	$(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestE2EApp

test-query-app: $(BINS)
	 $(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestQueryApp

test-k8s-integration:
	# ASSUMES:
	# 1. cluster created - `kind create cluster`
	# 2. cluster setup   - ./script/setup-kind.sh
	go test -v -tags k8s_integration ./pkg/apis/akash.network/v1
	go test -v -tags k8s_integration ./provider/cluster/kube

gentypes: $(PROTOC_FILES)

vendor:
	go mod vendor

kubetypes-deps-install:
	if [ -d "$(shell go env GOPATH)/src/k8s.io/code-generator" ]; then    \
		cd "$(shell go env GOPATH)/src/k8s.io/code-generator" && git pull;  \
		exit 0;                                                             \
	fi;                                                                   \
	mkdir -p "$(shell go env GOPATH)/src/k8s.io" && \
	git clone                                       \
	  git@github.com:kubernetes/code-generator.git  \
		"$(shell go env GOPATH)/src/k8s.io/code-generator"

kubetypes:
	chmod +x vendor/k8s.io/code-generator/generate-groups.sh
	vendor/k8s.io/code-generator/generate-groups.sh all \
  	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
  	akash.network:v1

mocks:
	mockery -case=underscore -dir provider              -output provider/mocks              -name StatusClient
	mockery -case=underscore -dir provider              -output provider/mocks              -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name ReadClient
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name Client
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name StatusClient

gofmt:
	find . -not -path './vendor*' -name '*.go' -type f | \
		xargs gofmt -s -w

clean: tools-clean
	rm -f $(BINS) $(IMAGE_BINS)

.PHONY: all bins build \
	akash \
	image image-bins \
	test test-nocache test-full test-coverage \
	deps-install devdeps-install \
	test-integraion \
	test-lint lintdeps-install \
	test-k8s-integration \
	test-vet \
	vendor \
	mocks \
	gofmt \
	docs \
	clean \
	kubetypes kubetypes-deps-install \
	install

###############################################################################
###                           Simulation                                    ###
###############################################################################

test-sim-fullapp:
	echo "Running app simulation test..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) ${APP_DIR} -run=TestFullAppSimulation -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-nondeterminism:
	echo "Running non-determinism test. This may take several minutes..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run TestAppStateDeterminism -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Period=0 -v -timeout 24h

test-sim-import-export:
	echo "Running application import/export simulation..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run=TestAppImportExport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-after-import:
	echo "Running application simulation-after-import..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run=TestAppSimulationAfterImport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sims: test-sim-fullapp test-sim-nondeterminism test-sim-import-export test-sim-after-import

###############################################################################
###                           Protobuf                                    ###
###############################################################################
UNAME_OS       := $(shell uname -s)
UNAME_ARCH     := $(shell uname -m)
CACHE_BASE     ?= $(abspath .cache)
CACHE          := $(CACHE_BASE)
CACHE_BIN      := $(CACHE)/bin
CACHE_INCLUDE  := $(CACHE)/include
BUF_VERSION    ?= 0.20.5
PROTOC_VERSION ?= 3.13.0
PROTOC_GRPC_GATEWAY_VERSION ?= 1.14.7

ifeq ($(UNAME_OS),Linux)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
  PROTOC_GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${PROTOC_GRPC_GATEWAY_VERSION}-linux-x86_64
endif
ifeq ($(UNAME_OS),Darwin)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
  PROTOC_GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${PROTOC_GRPC_GATEWAY_VERSION}-darwin-x86_64
endif

# This is needed to allow versions to be added to Golang modules with go get

# BUF points to the marker file for the installed version.
#
# If BUF_VERSION is changed, the binary will be re-downloaded.
BUF    := $(CACHE_BIN)/buf
PROTOC := $(CACHE_BIN)/protoc

proto-gen: $(PROTOC)
	./script/protocgen.sh

proto-lint: $(BUF)
	$(BUF) check lint --error-format=json

proto-check-breaking: $(BUF)
	$(BUF) check breaking --against-input '.git#branch=master'

TM_URL           = https://raw.githubusercontent.com/tendermint/tendermint/v0.34.0-rc3/proto/tendermint
GOGO_PROTO_URL   = https://raw.githubusercontent.com/regen-network/protobuf/cosmos
COSMOS_PROTO_URL = https://raw.githubusercontent.com/regen-network/cosmos-proto/master
COSMOS_SDK_PROTO_URL = https://raw.githubusercontent.com/cosmos/cosmos-sdk/master/proto/cosmos/base

TM_CRYPTO_TYPES     = third_party/proto/tendermint/crypto
TM_ABCI_TYPES       = third_party/proto/tendermint/abci
TM_TYPES     	    = third_party/proto/tendermint/types
TM_VERSION 			= third_party/proto/tendermint/version
TM_LIBS				= third_party/proto/tendermint/libs/bits

GOGO_PROTO_TYPES    = third_party/proto/gogoproto
COSMOS_PROTO_TYPES  = third_party/proto/cosmos_proto

SDK_ABCI_TYPES  	= third_party/proto/cosmos/base/abci/v1beta1
SDK_QUERY_TYPES  	= third_party/proto/cosmos/base/query/v1beta1
SDK_COIN_TYPES  	= third_party/proto/cosmos/base/v1beta1

proto-update-deps:
	mkdir -p $(GOGO_PROTO_TYPES)
	curl -sSL $(GOGO_PROTO_URL)/gogoproto/gogo.proto > $(GOGO_PROTO_TYPES)/gogo.proto

	mkdir -p $(COSMOS_PROTO_TYPES)
	curl -sSL $(COSMOS_PROTO_URL)/cosmos.proto > $(COSMOS_PROTO_TYPES)/cosmos.proto

	mkdir -p $(TM_ABCI_TYPES)
	curl -sSL $(TM_URL)/abci/types.proto > $(TM_ABCI_TYPES)/types.proto

	mkdir -p $(TM_VERSION)
	curl -sSL $(TM_URL)/version/types.proto > $(TM_VERSION)/types.proto

	mkdir -p $(TM_TYPES)
	curl -sSL $(TM_URL)/types/types.proto > $(TM_TYPES)/types.proto
	curl -sSL $(TM_URL)/types/evidence.proto > $(TM_TYPES)/evidence.proto
	curl -sSL $(TM_URL)/types/params.proto > $(TM_TYPES)/params.proto

	mkdir -p $(TM_CRYPTO_TYPES)
	curl -sSL $(TM_URL)/crypto/proof.proto > $(TM_CRYPTO_TYPES)/proof.proto
	curl -sSL $(TM_URL)/crypto/keys.proto > $(TM_CRYPTO_TYPES)/keys.proto

	mkdir -p $(TM_LIBS)
	curl -sSL $(TM_URL)/libs/bits/types.proto > $(TM_LIBS)/types.proto

	mkdir -p $(SDK_ABCI_TYPES)
	curl -sSL $(COSMOS_SDK_PROTO_URL)/abci/v1beta1/abci.proto > $(SDK_ABCI_TYPES)/abci.proto

	mkdir -p $(SDK_QUERY_TYPES)
	curl -sSL $(COSMOS_SDK_PROTO_URL)/query/v1beta1/pagination.proto > $(SDK_QUERY_TYPES)/pagination.proto

	mkdir -p $(SDK_COIN_TYPES)
	curl -sSL $(COSMOS_SDK_PROTO_URL)/v1beta1/coin.proto > $(SDK_COIN_TYPES)/coin.proto

cache-setup:
	@mkdir -p $(CACHE_BIN)
	@mkdir -p $(CACHE_INCLUDE)

$(BUF):
	@echo "Installing protoc buf cli..."
	@rm -f $@
	@curl -sSL \
		"https://github.com/bufbuild/buf/releases/download/v$(BUF_VERSION)/buf-$(UNAME_OS)-$(UNAME_ARCH)" \
		-o "$(CACHE_BIN)/buf"
	@chmod +x "$(CACHE_BIN)/buf"

$(PROTOC):
	@echo "Installing protoc compiler..."
	@rm -f $@
	@(cd /tmp; \
	curl -sOL "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) bin/protoc; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) 'include/*'; \
	rm -f ${PROTOC_ZIP})

grpc-gateway:
	@echo "Installing protoc-gen-grpc-gateway..."
	@rm -f $@
	@curl -o "${GOBIN}/protoc-gen-grpc-gateway" -L \
	"https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v${PROTOC_GRPC_GATEWAY_VERSION}/${PROTOC_GRPC_GATEWAY_BIN}"
	chmod +x "${GOBIN}/protoc-gen-grpc-gateway"

protoc-swagger:
ifeq (, $(shell which protoc-gen-swagger))
	@echo "Installing protoc-gen-swagger..."
	@go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
	@npm install -g swagger-combine
else
	@echo "protoc-gen-swagger already installed; skipping..."
endif

.PHONY: proto-tools
proto-tools: cache-setup $(BUF) $(PROTOC) grpc-gateway protoc-swagger

tools-clean:
	rm -rf $(CACHE)

proto-swagger-gen:
	./script/protoc-swagger-gen.sh

update-swagger-docs:
	statik -src=client/grpc-gateway -dest=client/grpc-gateway -f -m
	@if [ -n "$(git status --porcelain)" ]; then \
        echo "\033[91mSwagger docs are out of sync!!!\033[0m";\
        exit 1;\
    else \
    	echo "\033[92mSwagger docs are in sync\033[0m";\
    fi