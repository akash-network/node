BINS                  := akash
APP_DIR               := ./app

GO                    := GO111MODULE=on go
GOBIN                 := $(shell go env GOPATH)/bin

KIND_APP_IP           ?= $(shell make -sC _run/kube kind-k8s-ip)
KIND_APP_PORT         ?= $(shell make -sC _run/kube app-http-port)
KIND_VARS             ?= KIND_APP_IP="$(KIND_APP_IP)" KIND_APP_PORT="$(KIND_APP_PORT)"

UNAME_OS              := $(shell uname -s)
UNAME_ARCH            := $(shell uname -m)
CACHE_BASE            ?= $(abspath .cache)
CACHE                 := $(CACHE_BASE)
CACHE_BIN             := $(CACHE)/bin
CACHE_INCLUDE         := $(CACHE)/include
CACHE_VERSIONS        := $(CACHE)/versions

BUF_VERSION           ?= 0.20.5
PROTOC_VERSION        ?= 3.13.0
GRPC_GATEWAY_VERSION  ?= 1.14.7
GOLANGCI_LINT_VERSION ?= v1.27.0
GOLANG_VERSION        ?= 1.15.2
GOLANG_CROSS_VERSION  := v$(GOLANG_VERSION)

# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
BUF_VERSION_FILE           = $(CACHE_VERSIONS)/buf/$(BUF_VERSION)
PROTOC_VERSION_FILE        = $(CACHE_VERSIONS)/protoc/$(PROTOC_VERSION)
GRPC_GATEWAY_VERSION_FILE  = $(CACHE_VERSIONS)/protoc-gen-grpc-gateway/$(GRPC_GATEWAY_VERSION)
GOLANGCI_LINT_VERSION_FILE = $(CACHE_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)

GOLANGCI_LINT          = $(CACHE_BIN)/golangci-lint
LINT                   = $(GOLANGCI_LINT) run ./... --disable-all --deadline=5m --enable
MODVENDOR              = $(CACHE_BIN)/modvendor
BUF                   := $(CACHE_BIN)/buf
PROTOC                := $(CACHE_BIN)/protoc
GRPC_GATEWAY          := $(CACHE_BIN)/protoc-gen-grpc-gateway

# BUILD_TAGS are for builds withing this makefile
# GORELEASER_BUILD_TAGS are for goreleaser only
# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifeq ($(MAINNET),true)
	BUILD_MAINNET=mainnet
	BUILD_TAGS=netgo,ledger,mainnet
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS)
else
	BUILD_TAGS=netgo,ledger
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS),testnet
endif

GORELEASER_FLAGS    = -tags="$(GORELEASER_BUILD_TAGS)"
GORELEASER_LD_FLAGS = '-s -w -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(GORELEASER_BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags --abbrev=0) \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')'

BUILD_FLAGS = -mod=readonly -tags "$(BUILD_TAGS)" -ldflags '-X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(BUILD_TAGS)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')'

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

.PHONY: all
all: build bins

clean: cache-clean
	rm -f $(BINS)
