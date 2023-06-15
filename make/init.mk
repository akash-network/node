UNAME_OS              := $(shell uname -s)
UNAME_ARCH            := $(shell uname -m)

# certain targets need to use bash
# detect where bash is installed
# use akash-node-ready target as example
BASH_PATH := $(shell which bash)

ifeq (, $(shell which direnv))
$(warning "No direnv in $(PATH), consider installing. https://direnv.net")
endif

# AKASH_ROOT may not be set if environment does not support/use direnv
# in this case define it manually as well as all required env variables
ifndef AKASH_ROOT
	AKASH_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../)
	include $(AKASH_ROOT)/.env

	AKASH               := $(AKASH_DEVCACHE_BIN)/akash
	# setup .cache bins first in paths to have precedence over already installed same tools for system wide use
	PATH                := $(AKASH_DEVCACHE_BIN):$(AKASH_DEVCACHE_NODE_BIN):$(PATH)
endif

# require go<major>.<minor> to be equal
GO_MIN_REQUIRED              := $(shell echo $(GOLANG_VERSION) | cut -f1-2 -d ".")
DETECTED_GO_VERSION          := $(shell go version | cut -d ' ' -f 3 |  sed 's/go*//' | cut -f1-2 -d ".")
STRIPPED_GO_VERSION          := $(shell echo $(DETECTED_GO_VERSION) | cut -f1-2 -d ".")
__IS_GO_UPTODATE             := $(shell $(ROOT_DIR)/script/semver.sh compare $(STRIPPED_GO_VERSION) $(GO_MIN_REQUIRED) && echo $?)
GO_MOD_VERSION               := $(shell go mod edit -json | jq -r .Go | cut -f1-2 -d ".")
__IS_GO_MOD_MATCHING         := $(shell $(ROOT_DIR)/script/semver.sh compare $(GO_MOD_VERSION) $(GO_MIN_REQUIRED) && echo $?)

ifneq (0, $(__IS_GO_MOD_MATCHING))
$(error go version $(GO_MOD_VERSION) from go.mod does not match GO_MIN_REQUIRED=$(GO_MIN_REQUIRED))
endif

ifneq (0, $(__IS_GO_UPTODATE))
$(error invalid go$(DETECTED_GO_VERSION) version. installed must be == $(GO_MIN_REQUIRED))
else
$(info using go$(DETECTED_GO_VERSION))
endif

BINS                         := $(AKASH)

GOWORK                       ?= on

GO_MOD                       ?= vendor
ifeq ($(GOWORK), on)
GO_MOD                       := readonly
endif

GO                           := GO111MODULE=$(GO111MODULE) go
GO_BUILD                     := $(GO) build -mod=$(GO_MOD)
GO_TEST                      := $(GO) test -mod=$(GO_MOD)
GO_VET                       := $(GO) vet -mod=$(GO_MOD)

GO_MOD_NAME                  := $(shell go list -m 2>/dev/null)

# ==== Build tools versions ====
# Format <TOOL>_VERSION
GOLANGCI_LINT_VERSION        ?= v1.51.2
GOLANG_VERSION               ?= 1.16.1
STATIK_VERSION               ?= v0.1.7
GIT_CHGLOG_VERSION           ?= v0.15.1
MOCKERY_VERSION              ?= 2.24.0
COSMOVISOR_VERSION           ?= v1.4.0

# ==== Build tools version tracking ====
# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
GIT_CHGLOG_VERSION_FILE          := $(AKASH_DEVCACHE_VERSIONS)/git-chglog/$(GIT_CHGLOG_VERSION)
MOCKERY_VERSION_FILE             := $(AKASH_DEVCACHE_VERSIONS)/mockery/v$(MOCKERY_VERSION)
GOLANGCI_LINT_VERSION_FILE       := $(AKASH_DEVCACHE_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)
STATIK_VERSION_FILE              := $(AKASH_DEVCACHE_VERSIONS)/statik/$(STATIK_VERSION)
COSMOVISOR_VERSION_FILE          := $(AKASH_DEVCACHE_VERSIONS)/cosmovisor/$(COSMOVISOR_VERSION)

# ==== Build tools executables ====
GIT_CHGLOG                       := $(AKASH_DEVCACHE_BIN)/git-chglog
MOCKERY                          := $(AKASH_DEVCACHE_BIN)/mockery
NPM                              := npm
GOLANGCI_LINT                    := $(AKASH_DEVCACHE_BIN)/golangci-lint
STATIK                           := $(AKASH_DEVCACHE_BIN)/statik
COSMOVISOR                       := $(AKASH_DEVCACHE_BIN)/cosmovisor

include $(AKASH_ROOT)/make/setup-cache.mk
