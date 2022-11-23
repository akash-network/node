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
__IS_GO_UPTODATE             := $(shell ./script/semver.sh compare $(STRIPPED_GO_VERSION) $(GO_MIN_REQUIRED) && echo $?)
GO_MOD_VERSION               := $(shell go mod edit -json | jq -r .Go | cut -f1-2 -d ".")
__IS_GO_MOD_MATCHING         := $(shell ./script/semver.sh compare $(GO_MOD_VERSION) $(GO_MIN_REQUIRED) && echo $?)

ifneq (0, $(__IS_GO_MOD_MATCHING))
$(error go version $(GO_MOD_VERSION) from go.mod does not match GO_MIN_REQUIRED=$(GO_MIN_REQUIRED))
endif

ifneq (0, $(__IS_GO_UPTODATE))
$(error invalid go$(DETECTED_GO_VERSION) version. installed must be >= $(GO_MIN_REQUIRED))
else
$(info using go$(DETECTED_GO_VERSION))
endif

BINS                         := $(AKASH)

GO                           := GO111MODULE=$(GO111MODULE) go
GO_MOD_NAME                  := $(shell go list -m 2>/dev/null)

# ==== Build tools versions ====
# Format <TOOL>_VERSION
BUF_VERSION                  ?= 0.35.1
PROTOC_VERSION               ?= 3.13.0
PROTOC_GEN_GOCOSMOS_VERSION  ?= v0.3.1
GRPC_GATEWAY_VERSION         := $(shell $(GO) list -mod=readonly -m -f '{{ .Version }}' github.com/grpc-ecosystem/grpc-gateway)
PROTOC_SWAGGER_GEN_VERSION   := $(GRPC_GATEWAY_VERSION)
GOLANGCI_LINT_VERSION        ?= v1.50.0
GOLANG_VERSION               ?= 1.16.1
STATIK_VERSION               ?= v0.1.7
GIT_CHGLOG_VERSION           ?= v0.15.1
MODVENDOR_VERSION            ?= v0.3.0
MOCKERY_VERSION              ?= 2.12.1

# ==== Build tools version tracking ====
# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
PROTOC_VERSION_FILE              := $(AKASH_DEVCACHE_VERSIONS)/protoc/$(PROTOC_VERSION)
GRPC_GATEWAY_VERSION_FILE        := $(AKASH_DEVCACHE_VERSIONS)/protoc-gen-grpc-gateway/$(GRPC_GATEWAY_VERSION)
PROTOC_GEN_GOCOSMOS_VERSION_FILE := $(AKASH_DEVCACHE_VERSIONS)/protoc-gen-gocosmos/$(PROTOC_GEN_GOCOSMOS_VERSION)
STATIK_VERSION_FILE              := $(AKASH_DEVCACHE_VERSIONS)/statik/$(STATIK_VERSION)
MODVENDOR_VERSION_FILE           := $(AKASH_DEVCACHE_VERSIONS)/modvendor/$(MODVENDOR_VERSION)
GIT_CHGLOG_VERSION_FILE          := $(AKASH_DEVCACHE_VERSIONS)/git-chglog/$(GIT_CHGLOG_VERSION)
MOCKERY_VERSION_FILE             := $(AKASH_DEVCACHE_VERSIONS)/mockery/v$(MOCKERY_VERSION)
GOLANGCI_LINT_VERSION_FILE       := $(AKASH_DEVCACHE_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)

# ==== Build tools executables ====
MODVENDOR                        := $(AKASH_DEVCACHE_BIN)/modvendor
SWAGGER_COMBINE                  := $(AKASH_DEVCACHE_NODE_BIN)/swagger-combine
PROTOC_SWAGGER_GEN               := $(AKASH_DEVCACHE_BIN)/protoc-swagger-gen
PROTOC                           := $(AKASH_DEVCACHE_BIN)/protoc
STATIK                           := $(AKASH_DEVCACHE_BIN)/statik
PROTOC_GEN_GOCOSMOS              := $(AKASH_DEVCACHE_BIN)/protoc-gen-gocosmos
GRPC_GATEWAY                     := $(AKASH_DEVCACHE_BIN)/protoc-gen-grpc-gateway
GIT_CHGLOG                       := $(AKASH_DEVCACHE_BIN)/git-chglog
MOCKERY                          := $(AKASH_DEVCACHE_BIN)/mockery
NPM                              := npm
GOLANGCI_LINT                    := $(AKASH_DEVCACHE_BIN)/golangci-lint

include $(AKASH_ROOT)/make/setup-cache.mk
