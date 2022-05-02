# AKASH_ROOT may not be set if environment does not support/use direnv
# in this case define it manually as well as all required env variables
ifndef AKASH_ROOT
AKASH_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../)
include $(AKASH_ROOT)/.env
endif

AKASH                       = $(AKASH_DEVCACHE_BIN)/akash

BINS                       := $(AKASH)

GO                         := GO111MODULE=$(GO111MODULE) go

# setup .cache bins first in paths to have precedence over already installed same tools for system wide use
PATH                       := "$(PATH):$(AKASH_DEVCACHE_BIN):$(AKASH_DEVCACHE_NODE_BIN)"

BUF_VERSION                  ?= 0.35.1
PROTOC_VERSION               ?= 3.13.0
PROTOC_GEN_GOCOSMOS_VERSION  ?= v0.3.1
GRPC_GATEWAY_VERSION         := $(shell $(GO) list -mod=readonly -m -f '{{ .Version }}' github.com/grpc-ecosystem/grpc-gateway)
PROTOC_SWAGGER_GEN_VERSION   := $(GRPC_GATEWAY_VERSION)
GOLANGCI_LINT_VERSION        ?= v1.45.2
GOLANG_VERSION               ?= 1.16.1
STATIK_VERSION               ?= v0.1.7
GIT_CHGLOG_VERSION           ?= v0.15.1
MODVENDOR_VERSION            ?= v0.3.0
MOCKERY_VERSION              ?= 2.12.1
K8S_CODE_GEN_VERSION         ?= v0.19.3

# <TOOL>_VERSION_FILE points to the marker file for the installed version.
# If <TOOL>_VERSION_FILE is changed, the binary will be re-downloaded.
PROTOC_VERSION_FILE              := $(AKASH_DEVCACHE_VERSIONS)/protoc/$(PROTOC_VERSION)
GRPC_GATEWAY_VERSION_FILE        := $(AKASH_DEVCACHE_VERSIONS)/protoc-gen-grpc-gateway/$(GRPC_GATEWAY_VERSION)
PROTOC_GEN_GOCOSMOS_VERSION_FILE := $(AKASH_DEVCACHE_VERSIONS)/protoc-gen-gocosmos/$(PROTOC_GEN_GOCOSMOS_VERSION)
STATIK_VERSION_FILE              := $(AKASH_DEVCACHE_VERSIONS)/statik/$(STATIK_VERSION)
MODVENDOR_VERSION_FILE           := $(AKASH_DEVCACHE_VERSIONS)/modvendor/$(MODVENDOR_VERSION)
GIT_CHGLOG_VERSION_FILE          := $(AKASH_DEVCACHE_VERSIONS)/git-chglog/$(GIT_CHGLOG_VERSION)
MOCKERY_VERSION_FILE             := $(AKASH_DEVCACHE_VERSIONS)/mockery/v$(MOCKERY_VERSION)
K8S_CODE_GEN_VERSION_FILE        := $(AKASH_DEVCACHE_VERSIONS)/k8s-codegen/$(K8S_CODE_GEN_VERSION)
GOLANGCI_LINT_VERSION_FILE       := $(AKASH_DEVCACHE_VERSIONS)/golangci-lint/$(GOLANGCI_LINT_VERSION)

MODVENDOR                         = $(AKASH_DEVCACHE_BIN)/modvendor
SWAGGER_COMBINE                   = $(AKASH_DEVCACHE_NODE_BIN)/swagger-combine
PROTOC_SWAGGER_GEN               := $(AKASH_DEVCACHE_BIN)/protoc-swagger-gen
PROTOC                           := $(AKASH_DEVCACHE_BIN)/protoc
STATIK                           := $(AKASH_DEVCACHE_BIN)/statik
PROTOC_GEN_GOCOSMOS              := $(AKASH_DEVCACHE_BIN)/protoc-gen-gocosmos
GRPC_GATEWAY                     := $(AKASH_DEVCACHE_BIN)/protoc-gen-grpc-gateway
GIT_CHGLOG                       := $(AKASH_DEVCACHE_BIN)/git-chglog
MOCKERY                          := $(AKASH_DEVCACHE_BIN)/mockery
K8S_GENERATE_GROUPS              := $(AKASH_ROOT)/vendor/k8s.io/code-generator/generate-groups.sh
K8S_GO_TO_PROTOBUF               := $(AKASH_DEVCACHE_BIN)/go-to-protobuf
KIND                             := kind
NPM                              := npm
GOLANGCI_LINT                    := $(AKASH_DEVCACHE_BIN)/golangci-lint

include $(AKASH_ROOT)/make/setup-cache.mk
