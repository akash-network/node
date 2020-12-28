BINS                  := akash
APP_DIR               := ./app

GO                    := GO111MODULE=on go
GOBIN                 := $(shell go env GOPATH)/bin

KIND_APP_IP           ?= $(shell make -sC _run/kube kind-k8s-ip)
KIND_APP_PORT         ?= $(shell make -sC _run/kube app-http-port)
KIND_VARS             ?= KUBE_INGRESS_IP="$(KIND_APP_IP)" KUBE_INGRESS_PORT="$(KIND_APP_PORT)"

UNAME_OS              := $(shell uname -s)
UNAME_ARCH            := $(shell uname -m)
CACHE_BASE            ?= $(abspath .cache)
CACHE                 := $(CACHE_BASE)
CACHE_BIN             := $(CACHE)/bin
CACHE_INCLUDE         := $(CACHE)/include
CACHE_VERSIONS        := $(CACHE)/versions
CACHE_NODE_MODULES    := $(CACHE)/node_modules
CACHE_NODE_BIN        := $(CACHE_NODE_MODULES)/.bin

# setup .cache bins first in paths to have precedence over already installed same tools for system wide use
PATH                  := "$(PATH):$(CACHE_BIN):$(CACHE_NODE_BIN)"

BUF_VERSION                ?= 0.35.1
PROTOC_VERSION             ?= 3.13.0
PROTOC_GEN_COSMOS_VERSION  ?= master
GRPC_GATEWAY_VERSION       ?= 1.14.7
GOLANGCI_LINT_VERSION      ?= v1.31.0
GOLANG_VERSION             ?= 1.15.2
GOLANG_CROSS_VERSION       := v$(GOLANG_VERSION)
STATIK_VERSION             ?= v0.1.7

# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
PROTOC_VERSION_FILE             = $(CACHE_VERSIONS)/protoc/$(PROTOC_VERSION)
GRPC_GATEWAY_VERSION_FILE       = $(CACHE_VERSIONS)/protoc-gen-grpc-gateway/$(GRPC_GATEWAY_VERSION)
PROTOC_GEN_COSMOS_VERSION_FILE  = $(CACHE_VERSIONS)/protoc-gen-cosmos/$(PROTOC_GEN_COSMOS_VERSION)
STATIK_VERSION_FILE             = $(CACHE_VERSIONS)/statik/$(STATIK_VERSION)
MODVENDOR                       = $(CACHE_BIN)/modvendor
SWAGGER_COMBINE                 = $(CACHE_NODE_BIN)/swagger-combine
PROTOC_SWAGGER_GEN             := $(CACHE_BIN)/protoc-swagger-gen
PROTOC                         := $(CACHE_BIN)/protoc
STATIK                         := $(CACHE_BIN)/statik
PROTOC_GEN_COSMOS              := $(CACHE_BIN)/protoc-gen-cosmos
GRPC_GATEWAY                   := $(CACHE_BIN)/protoc-gen-grpc-gateway

DOCKER_RUN            := docker run --rm -v $(shell pwd):/workspace -w /workspace
DOCKER_BUF            := $(DOCKER_RUN) bufbuild/buf:$(BUF_VERSION)
DOCKER_CLANG          := $(DOCKER_RUN) tendermintdev/docker-build-proto
GOLANGCI_LINT          = $(DOCKER_RUN) golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)-alpine golangci-lint run
LINT                   = $(GOLANGCI_LINT) ./... --disable-all --deadline=5m --enable
TEST_DOCKER_REPO      := jackzampolin/akashtest

GORELEASER_CONFIG      = .goreleaser.yaml

# BUILD_TAGS are for builds withing this makefile
# GORELEASER_BUILD_TAGS are for goreleaser only
# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifeq ($(MAINNET),true)
	BUILD_MAINNET=mainnet
	BUILD_TAGS=netgo,ledger,mainnet
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS)
	GORELEASER_HOMEBREW_NAME=akash
	GORELEASER_HOMEBREW_CUSTOM=
else
	BUILD_TAGS=netgo,ledger
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS),testnet
	GORELEASER_HOMEBREW_NAME="akash-edge"
	GORELEASER_HOMEBREW_CUSTOM=keg_only :unneeded, \"This is testnet release. Run brew install ovrclk/tap/akash to install mainnet version\"
endif

GORELEASER_FLAGS    = -tags="$(GORELEASER_BUILD_TAGS)"
GORELEASER_LD_FLAGS = -s -w -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X github.com/cosmos/cosmos-sdk/version.BuildTags="$(GORELEASER_BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags --abbrev=0) \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')

ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')

# check for nostrip option
ifeq (,$(findstring nostrip,$(BUILD_OPTIONS)))
	ldflags += -s -w
endif
ldflags += $(LDFLAGS)
ldflags := $(strip $(ldflags))

BUILD_FLAGS := -mod=readonly -tags "$(BUILD_TAGS)" -ldflags '$(ldflags)'
# check for nostrip option
ifeq (,$(findstring nostrip,$(BUILD_OPTIONS)))
	BUILD_FLAGS += -trimpath
endif

.PHONY: all
all: build bins

.PHONY: clean
clean: cache-clean
	rm -f $(BINS)

include make/proto.mk
include make/setup-cache.mk
include make/releasing.mk
include make/mod.mk
include make/lint.mk
include make/test-integration.mk
include make/test-simulation.mk
include make/tools.mk
include make/environment.mk
include make/codegen.mk

test-docker:
	@docker build -f _build/Dockerfile.test -t ${TEST_DOCKER_REPO}:$(shell git rev-parse --short HEAD) .
	@docker tag ${TEST_DOCKER_REPO}:$(shell git rev-parse --short HEAD) ${TEST_DOCKER_REPO}:$(shell git rev-parse --abbrev-ref HEAD | sed 's#/#_#g')
	@docker tag ${TEST_DOCKER_REPO}:$(shell git rev-parse --short HEAD) ${TEST_DOCKER_REPO}:latest

test-docker-push: test-docker
	@docker push ${TEST_DOCKER_REPO}:$(shell git rev-parse --short HEAD)
	@docker push ${TEST_DOCKER_REPO}:$(shell git rev-parse --abbrev-ref HEAD | sed 's#/#_#g')
	@docker push ${TEST_DOCKER_REPO}:latest
