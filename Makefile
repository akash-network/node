APP_DIR               := ./app

GOBIN                 := $(shell go env GOPATH)/bin

KIND_APP_IP           ?= $(shell make -sC _run/kube kind-k8s-ip)
KIND_APP_PORT         ?= $(shell make -sC _run/kube app-http-port)
KIND_VARS             ?= KUBE_INGRESS_IP="$(KIND_APP_IP)" KUBE_INGRESS_PORT="$(KIND_APP_PORT)"

UNAME_OS              := $(shell uname -s)
UNAME_ARCH            := $(shell uname -m)

include make/init.mk

.DEFAULT_GOAL         := $(AKASH)

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

GORELEASER_TAG       ?= $(shell git describe --tags --abbrev=0)
GORELEASER_IS_PREREL ?= $(shell $(ROOT_DIR)/script/is_prerelease.sh "$(GORELEASER_TAG)")

# BUILD_TAGS are for builds withing this makefile
# GORELEASER_BUILD_TAGS are for goreleaser only
# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifeq ($(MAINNET),true)
	BUILD_MAINNET=mainnet
	BUILD_TAGS=osusergo,netgo,ledger,mainnet,static_build
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS)
else
	BUILD_TAGS=osusergo,netgo,ledger,static_build
	GORELEASER_BUILD_TAGS=$(BUILD_TAGS),testnet
endif

ifeq ($(GORELEASER_IS_PREREL),false)
	GORELEASER_HOMEBREW_NAME=akash
	GORELEASER_HOMEBREW_CUSTOM=
else
	GORELEASER_HOMEBREW_NAME="akash-edge"
	GORELEASER_HOMEBREW_CUSTOM=keg_only :unneeded, \"This is testnet release. Run brew install ovrclk/tap/akash to install mainnet version\"
endif

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
include make/releasing.mk
include make/mod.mk
include make/lint.mk
include make/test-integration.mk
include make/test-simulation.mk
include make/tools.mk
include make/environment.mk
include make/codegen.mk
