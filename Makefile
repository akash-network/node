APP_DIR               := ./app

GOBIN                 ?= $(shell go env GOPATH)/bin

include make/init.mk

.DEFAULT_GOAL         := bins

DOCKER_RUN            := docker run --rm -v $(shell pwd):/workspace -w /workspace
GOLANGCI_LINT_RUN     := $(GOLANGCI_LINT) run
LINT                   = $(GOLANGCI_LINT_RUN) ./... --disable-all --deadline=5m --enable

GORELEASER_CONFIG     ?= .goreleaser.yaml

GIT_HEAD_COMMIT_LONG  := $(shell git log -1 --format='%H')
GIT_HEAD_COMMIT_SHORT := $(shell git rev-parse --short HEAD)
GIT_HEAD_ABBREV       := $(shell git rev-parse --abbrev-ref HEAD)

IS_PREREL             := $(shell $(ROOT_DIR)/script/is_prerelease.sh "$(RELEASE_TAG)" && echo "true" || echo "false")
IS_MAINNET            := $(shell $(ROOT_DIR)/script/mainnet-from-tag.sh "$(RELEASE_TAG)" && echo "true" || echo "false")
IS_STABLE             ?= false

GO_LINKMODE            ?= external
GOMOD                  ?= readonly
BUILD_TAGS             ?= osusergo,netgo,hidraw,ledger
GORELEASER_STRIP_FLAGS ?=


ifeq ($(IS_MAINNET), true)
	ifeq ($(IS_PREREL), false)
		IS_STABLE                  := true
	endif
endif

GOMOD                  ?= readonly

ifneq ($(UNAME_OS),Darwin)
BUILD_OPTIONS          ?= static-link
endif

BUILD_TAGS             := osusergo netgo ledger muslc gcc
DB_BACKEND             := goleveldb
BUILD_FLAGS            :=

GORELEASER_STRIP_FLAGS ?=

ifeq (cleveldb,$(findstring cleveldb,$(BUILD_OPTIONS)))
	DB_BACKEND=cleveldb
else ifeq (rocksdb,$(findstring rocksdb,$(BUILD_OPTIONS)))
	DB_BACKEND=rocksdb
else ifeq (goleveldb,$(findstring goleveldb,$(BUILD_OPTIONS)))
	DB_BACKEND=goleveldb
endif

ifneq (,$(findstring cgotrace,$(BUILD_OPTIONS)))
	BUILD_TAGS += cgotrace
endif

build_tags    := $(strip $(BUILD_TAGS))
build_tags_cs := $(subst $(WHITESPACE),$(COMMA),$(build_tags))

ldflags := -X github.com/cosmos/cosmos-sdk/version.Name=akash \
-X github.com/cosmos/cosmos-sdk/version.AppName=akash \
-X github.com/cosmos/cosmos-sdk/version.BuildTags="$(build_tags_cs)" \
-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
-X github.com/cosmos/cosmos-sdk/version.Commit=$(GIT_HEAD_COMMIT_LONG) \
-X github.com/cosmos/cosmos-sdk/types.DBBackend=$(DB_BACKEND)

GORELEASER_LDFLAGS := $(ldflags)

ldflags += -linkmode=external

ifeq (static-link,$(findstring static-link,$(BUILD_OPTIONS)))
	ldflags += -extldflags "-L$(AKASH_DEVCACHE_LIB) -lm -Wl,-z,muldefs"
else
	ldflags += -extldflags "-L$(AKASH_DEVCACHE_LIB)"
endif

# check for nostrip option
ifeq (,$(findstring nostrip,$(BUILD_OPTIONS)))
	ldflags     += -s -w
	BUILD_FLAGS += -trimpath
endif

ifeq (delve,$(findstring delve,$(BUILD_OPTIONS)))
	BUILD_FLAGS += -gcflags "all=-N -l"
endif

ldflags += $(LDFLAGS)
ldflags := $(strip $(ldflags))

GORELEASER_TAGS  := $(BUILD_TAGS)
GORELEASER_FLAGS := $(BUILD_FLAGS) -mod=$(GOMOD) -tags='$(build_tags)'

BUILD_FLAGS += -mod=$(GOMOD) -tags='$(build_tags_cs)' -ldflags '$(ldflags)'

.PHONY: all
all: build bins

.PHONY: clean
clean: cache-clean
	rm -f $(BINS)

include make/cosmwasm.mk
include make/releasing.mk
include make/mod.mk
include make/lint.mk
include make/test-integration.mk
include make/test-simulation.mk
include make/tools.mk
include make/codegen.mk
