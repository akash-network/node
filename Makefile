BINS                  := akash
IMAGE_BINS            := _build/akash
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
GOLANG_CROSS_VERSION  ?= v1.15.2

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

TEST_IMAGE_BUILD_ENV   = CGO_ENABLED=1 GOOS=linux GOARCH=amd64

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

all: build bins

bins: $(BINS)

build:
	$(GO) build ./...

generate:
	$(GO) generate ./...

akash:
	$(GO) build $(BUILD_FLAGS) ./cmd/akash

install:
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

image-minikube:
	eval $$(minikube docker-env) && docker-image

shellcheck:
	docker run --rm \
	--volume ${PWD}:/shellcheck \
	--entrypoint sh \
	koalaman/shellcheck-alpine:stable \
	-x /shellcheck/script/shellcheck.sh

test:
	$(GO) test -tags=$(BUILD_MAINNET) -timeout 300s ./...

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
	$(GOLANGCI_LINT) run

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

# Golang modules and vendoring
deps-install:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

deps-vendor:
	go mod vendor

test-integration: $(BINS)
	cp akash ./_build
	go test -mod=readonly -p 4 -tags "integration $(BUILD_MAINNET)" -v ./integration/...

test-e2e-integration:
	# Assumes cluster created: `make -s -C _run/kube kind-cluster-create`
	$(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestIntegrationTestSuite

test-query-app:
	 $(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestQueryApp

test-k8s-integration:
	# ASSUMES:
	# 1. cluster created - `kind create cluster`
	# 2. cluster setup   - ./script/setup-kind.sh
	go test -v -tags k8s_integration ./pkg/apis/akash.network/v1
	go test -v -tags k8s_integration ./provider/cluster/kube

gentypes: $(PROTOC_FILES)

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

clean: cache-clean
	rm -f $(BINS) $(IMAGE_BINS)

.PHONY: all bins build \
	akash \
	image image-bins \
	test test-nocache test-full test-coverage \
	test-integraion \
	test-lint \
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
ifeq ($(UNAME_OS),Linux)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
  GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-linux-x86_64
endif
ifeq ($(UNAME_OS),Darwin)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
  GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-darwin-x86_64
endif

proto-gen: $(PROTOC) $(GRPC_GATEWAY) modvendor
	./script/protocgen.sh

proto-lint: $(BUF) modvendor
	$(BUF) check lint --error-format=json

proto-check-breaking: $(BUF) modvendor
	$(BUF) check breaking --against-input '.git#branch=master'

GOOGLE_API_PROTO_URL = https://raw.githubusercontent.com/googleapis/googleapis/master/google/api
GOOGLE_PROTO_TYPES   = $(CACHE_INCLUDE)/google/api

.PHONY: modvendor
modvendor: modsensure $(MODVENDOR)
	@echo "vendoring non-go files files..."
	@echo "vendoring *.proto files..."
	$(MODVENDOR) -copy="**/*.proto" -include=\
github.com/cosmos/cosmos-sdk/proto,\
github.com/tendermint/tendermint/proto,\
github.com/gogo/protobuf,\
github.com/regen-network/cosmos-proto/cosmos.proto
	rm -rf $(GOOGLE_PROTO_TYPES)
	mkdir -p $(GOOGLE_PROTO_TYPES)
	curl -sSL $(GOOGLE_API_PROTO_URL)/http.proto > $(GOOGLE_PROTO_TYPES)/http.proto
	curl -sSL $(GOOGLE_API_PROTO_URL)/annotations.proto > $(GOOGLE_PROTO_TYPES)/annotations.proto
	curl -sSL $(GOOGLE_API_PROTO_URL)/httpbody.proto > $(GOOGLE_PROTO_TYPES)/httpbody.proto
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=\
github.com/zondax/hid

# Tools installation
$(CACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(CACHE_BIN)
	mkdir -p $(CACHE_INCLUDE)
	mkdir -p $(CACHE_VERSIONS)

$(GOLANGCI_LINT_VERSION_FILE): $(CACHE)
	@echo "installing golangci-lint..."
	rm -f $(GOLANGCI_LINT)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(CACHE_BIN) $(GOLANGCI_LINT_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GOLANGCI_LINT): $(GOLANGCI_LINT_VERSION_FILE)

.PHONY:lintdeps-install
lintdeps-install: $(GOLANGCI_LINT)
	@echo "lintdeps-install is deprecated and will be removed once Github Actions migrated to use .cache/bin/golangci-lint"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION)

$(BUF_VERSION_FILE): $(CACHE)
	@echo "installing protoc buf cli..."
	rm -f $(BUF)
	curl -sSL \
		"https://github.com/bufbuild/buf/releases/download/v$(BUF_VERSION)/buf-$(UNAME_OS)-$(UNAME_ARCH)" \
		-o "$(CACHE_BIN)/buf"
	chmod +x "$(CACHE_BIN)/buf"
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(BUF): $(BUF_VERSION_FILE)

$(PROTOC_VERSION_FILE): $(CACHE)
	@echo "installing protoc compiler..."
	rm -f $(PROTOC)
	(cd /tmp; \
	curl -sOL "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) bin/protoc; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) 'include/*'; \
	rm -f ${PROTOC_ZIP})
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(PROTOC): $(PROTOC_VERSION_FILE)

$(GRPC_GATEWAY_VERSION_FILE): $(CACHE)
	@echo "Installing protoc-gen-grpc-gateway..."
	rm -f $(GRPC_GATEWAY)
	curl -o "${CACHE_BIN}/protoc-gen-grpc-gateway" -L \
	"https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v${GRPC_GATEWAY_VERSION}/${GRPC_GATEWAY_BIN}"
	chmod +x "$(CACHE_BIN)/protoc-gen-grpc-gateway"
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GRPC_GATEWAY): $(GRPC_GATEWAY_VERSION_FILE)

$(MODVENDOR): $(CACHE)
	@echo "installing modvendor..."
	GOBIN=$(CACHE_BIN) GO111MODULE=off go get github.com/goware/modvendor

protoc-swagger:
ifeq (, $(shell which swagger-combine))
	@echo "Installing swagger-combine..."
	npm install -g swagger-combine
else
	@echo "swagger-combine already installed; skipping..."
endif

kubetypes-deps-install:
	if [ -d "$(shell go env GOPATH)/src/k8s.io/code-generator" ]; then    \
		cd "$(shell go env GOPATH)/src/k8s.io/code-generator" && git pull;  \
		exit 0;                                                             \
	fi;                                                                   \
	mkdir -p "$(shell go env GOPATH)/src/k8s.io" && \
	git clone  git@github.com:kubernetes/code-generator.git \
		"$(shell go env GOPATH)/src/k8s.io/code-generator"

devdeps-install: $(GOLANGCI_LINT) kubetypes-deps-install
	$(GO) install github.com/vektra/mockery/.../
	$(GO) install k8s.io/code-generator/...
	$(GO) install sigs.k8s.io/kind
	$(GO) install golang.org/x/tools/cmd/stringer
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger

cache-clean:
	rm -rf $(CACHE)

.PHONY: modsensure
modsensure: deps-tidy deps-vendor

.PHONY: codegen
codegen: generate proto-gen kubetypes

.PHONY: setup-devenv
setup-devenv: $(GOLANGCI_LINT) $(BUF) $(PROTOC) $(GRPC_GATEWAY) $(MODVENDOR) protoc-swagger deps-vendor modvendor

.PHONY: setup-cienv
setup-cienv: deps-vendor modvendor $(GOLANGCI_LINT)

proto-swagger-gen: protoc-swagger
	./script/protoc-swagger-gen.sh

update-swagger-docs: proto-swagger-gen
	statik -src=client/docs -dest=client/docs -f -m
	if [ -n "$(git status --porcelain)" ]; then \
        echo "\033[91mSwagger docs are out of sync!!!\033[0m";\
        exit 1;\
    else \
    	echo "\033[92mSwagger docs are in sync\033[0m";\
	fi

.PHONY: docker-imag
docker-image:
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		-f .goreleaser-docker.yaml --rm-dist --skip-validate --skip-publish --snapshot

.PHONY: release-dry-run
release-dry-run: modvendor
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		--rm-dist --skip-validate --skip-publish

.PHONY: release
release: modvendor
	@if [ ! -f ".release-env" ]; then \
		echo "\033[91m.release-env is required for release\033[0m";\
		exit 1;\
	fi
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		--env-file .release-env \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		release --rm-dist
