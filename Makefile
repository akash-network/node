BINS                  := akash akash_docgen
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

include .makerc

BUF_VERSION                ?= 0.35.1
PROTOC_VERSION             ?= 3.13.0
PROTOC_GEN_COSMOS_VERSION  ?= v0.3.1
PROTOC_SWAGGER_GEN_VERSION ?= v1.15.2
GRPC_GATEWAY_VERSION       ?= 1.14.7
GOLANGCI_LINT_VERSION      ?= v1.38.0
GOLANG_VERSION             ?= 1.16.1
GOLANG_CROSS_VERSION       := v$(GOLANG_VERSION)
STATIK_VERSION             ?= v0.1.7
GIT_CHGLOG_VERSION         ?= v0.10.0
MODVENDOR_VERSION          ?= v0.3.0
MOCKERY_VERSION            ?= 2.5.1

# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
PROTOC_VERSION_FILE             = $(CACHE_VERSIONS)/protoc/$(PROTOC_VERSION)
GRPC_GATEWAY_VERSION_FILE       = $(CACHE_VERSIONS)/protoc-gen-grpc-gateway/$(GRPC_GATEWAY_VERSION)
PROTOC_GEN_COSMOS_VERSION_FILE  = $(CACHE_VERSIONS)/protoc-gen-cosmos/$(PROTOC_GEN_COSMOS_VERSION)
STATIK_VERSION_FILE             = $(CACHE_VERSIONS)/statik/$(STATIK_VERSION)
MODVENDOR_VERSION_FILE          = $(CACHE_VERSIONS)/modvendor/$(MODVENDOR_VERSION)
GIT_CHGLOG_VERSION_FILE         = $(CACHE_VERSIONS)/git-chglog/$(GIT_CHGLOG_VERSION)
MOCKERY_VERSION_FILE            = $(CACHE_VERSIONS)/mockery/v$(MOCKERY_VERSION)

MODVENDOR                       = $(CACHE_BIN)/modvendor
SWAGGER_COMBINE                 = $(CACHE_NODE_BIN)/swagger-combine
PROTOC_SWAGGER_GEN             := $(CACHE_BIN)/protoc-swagger-gen
PROTOC                         := $(CACHE_BIN)/protoc
STATIK                         := $(CACHE_BIN)/statik
PROTOC_GEN_COSMOS              := $(CACHE_BIN)/protoc-gen-cosmos
GRPC_GATEWAY                   := $(CACHE_BIN)/protoc-gen-grpc-gateway
GIT_CHGLOG                     := $(CACHE_BIN)/git-chglog
MOCKERY                        := $(CACHE_BIN)/mockery

DOCKER_RUN            := docker run --rm -v $(shell pwd):/workspace -w /workspace
DOCKER_BUF            := $(DOCKER_RUN) bufbuild/buf:$(BUF_VERSION)
DOCKER_CLANG          := $(DOCKER_RUN) tendermintdev/docker-build-proto
GOLANGCI_LINT          = $(DOCKER_RUN) --network none golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)-alpine golangci-lint run
LINT                   = $(GOLANGCI_LINT) ./... --disable-all --deadline=5m --enable
TEST_DOCKER_REPO      := ovrclk/akashtest

GORELEASER_CONFIG      = .goreleaser.yaml

GIT_HEAD_COMMIT_LONG  := $(shell git log -1 --format='%H')
GIT_HEAD_COMMIT_SHORT := $(shell git rev-parse --short HEAD)
GIT_HEAD_ABBREV       := $(shell git rev-parse --abbrev-ref HEAD)

# BUILD_TAGS are for builds withing this makefile
# GORELEASER_BUILD_TAGS are for goreleaser only
# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifeq ($(MAINNET),true)
	BUILD_MAINNET=mainnet
	BUILD_TAGS=osusergo,netgo,ledger,mainnet,static_build
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS)
	GORELEASER_HOMEBREW_NAME=akash
	GORELEASER_HOMEBREW_CUSTOM=
else
	BUILD_TAGS=osusergo,netgo,ledger,static_build
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS),testnet
	GORELEASER_HOMEBREW_NAME="akash-edge"
	GORELEASER_HOMEBREW_CUSTOM=keg_only :unneeded, \"This is testnet release. Run brew install ovrclk/tap/akash to install mainnet version\"
endif

GORELEASER_TAG     ?= $(shell git describe --tags --abbrev=0)

GORELEASER_FLAGS    = -tags="$(GORELEASER_BUILD_TAGS)"
GORELEASER_LD_FLAGS = -s -w -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X github.com/cosmos/cosmos-sdk/version.BuildTags="$(GORELEASER_BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(GORELEASER_TAG) \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(GIT_HEAD_COMMIT_LONG)

ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(GIT_HEAD_COMMIT_LONG)

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
