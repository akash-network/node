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

BINS                         := $(AKASH)
SEMVER                       := $(AKASH_ROOT)/script/semver.sh

__local_go                   := $(shell GOTOOLCHAIN=local go version | cut -d ' ' -f 3 | sed 's/go*//' | tr -d '\n')
__is_local_go_satisfies      := $(shell $(SEMVER) compare "v$(__local_go)" "v1.20.7"; echo $?)

ifeq (-1, $(__is_local_go_satisfies))
$(error "unsupported local go$(__local_go) version . min required go1.21.0")
endif

GO_VERSION                   := $(shell go mod edit -json | jq -r .Go | tr -d '\n')
GOTOOLCHAIN                  := $(shell go mod edit -json | jq -r .Toolchain | tr -d '\n')
GOTOOLCHAIN_SEMVER           := v$(shell echo "$(GOTOOLCHAIN)" | sed 's/go*//' | tr -d '\n')

GO                           := GO111MODULE=$(GO111MODULE) go
GOWORK                       ?= on
GO_MOD                       ?= vendor
ifeq ($(GOWORK), on)
GO_MOD                       := readonly
endif

GO_BUILD                     := $(GO) build -mod=$(GO_MOD)
GO_TEST                      := $(GO) test -mod=$(GO_MOD)
GO_VET                       := $(GO) vet -mod=$(GO_MOD)
GO_MOD_NAME                  := $(shell go list -m 2>/dev/null)

ifeq ($(OS),Windows_NT)
	DETECTED_OS := Windows
else
	DETECTED_OS := $(shell sh -c 'uname 2>/dev/null || echo Unknown')
endif

ifeq ($(DETECTED_OS), Darwin)
	# on MacOS disable deprecation warnings security framework
	export CGO_CFLAGS=-Wno-deprecated-declarations

	# on MacOS Sonoma Beta there is a bit of discrepancy between Go and new prime linker
	clang_version := $(shell echo | clang -dM -E - | grep __clang_major__ | cut -d ' ' -f 3 | tr -d '\n')
	go_has_ld_fix := $(shell $(SEMVER) compare "v$(GOTOOLCHAIN_SEMVER)" "v1.22.0" | tr -d '\n')

	ifeq (15,$(clang_version))
		ifeq (-1,$(go_has_ld_fix))
			export CGO_LDFLAGS=-Wl,-ld_classic -Wno-deprecated-declarations
		endif
	endif
endif

# ==== Build tools versions ====
# Format <TOOL>_VERSION
GOLANGCI_LINT_VERSION        ?= v1.51.2
STATIK_VERSION               ?= v0.1.7
GIT_CHGLOG_VERSION           ?= v0.15.1
MOCKERY_VERSION              ?= 2.24.0
COSMOVISOR_VERSION           ?= v1.5.0

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
