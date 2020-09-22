BINS       := akashctl akashd
IMAGE_BINS := _build/akashctl _build/akashd
APP_DIR    := ./app

GO    := GO111MODULE=on go
GOBIN := $(shell go env GOPATH)/bin

KIND_APP_IP   ?= $(shell make -sC _run/kube kind-k8s-ip)
KIND_APP_PORT ?= $(shell make -sC _run/kube app-http-port)
KIND_VARS     ?= KIND_APP_IP="$(KIND_APP_IP)" KIND_APP_PORT="$(KIND_APP_PORT)"

CACHE_BASE            ?= $(abspath .cache)
CACHE                 := $(CACHE_BASE)
CACHE_BIN             := $(CACHE)/bin
CACHE_INCLUDE         := $(CACHE)/include
CACHE_VERSIONS        := $(CACHE)/versions
MODVENDOR              = $(CACHE_BIN)/modvendor

GOLANG_CROSS_VERSION  ?= v1.15.2
GOLANGCI_LINT_VERSION = v1.27.0

IMAGE_BUILD_ENV = GOOS=linux GOARCH=amd64

# Setting mainnet flag based on env value
# export MAINNET=true to set build tag mainnet
ifneq ($(MAINNET),false)
	BUILD_MAINNET=mainnet
	BUILD_TAGS=netgo,ledger,mainnet
else
	BUILD_TAGS=netgo,ledger
endif

BUILD_FLAGS = -mod=readonly -tags "$(BUILD_TAGS)" -ldflags \
 '-X github.com/cosmos/cosmos-sdk/version.Name=akash \
  -X github.com/cosmos/cosmos-sdk/version.ServerName=akashd \
  -X github.com/cosmos/cosmos-sdk/version.ClientName=akashctl \
  -X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(BUILD_TAGS)" \
  -X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
  -X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')'

all: build bins

bins: $(BINS)

build:
	$(GO) build ./...

generate:
	$(GO) generate ./...

akashctl:
	$(GO) build $(BUILD_FLAGS) ./cmd/akashctl

akashd:
	$(GO) build $(BUILD_FLAGS) ./cmd/akashd

image-bins:
	$(IMAGE_BUILD_ENV) $(GO) build $(BUILD_FLAGS) -o _build/akashctl  ./cmd/akashctl
	$(IMAGE_BUILD_ENV) $(GO) build $(BUILD_FLAGS) -o _build/akashd ./cmd/akashd

image: image-bins
	docker build --rm            \
		-t ovrclk/akash:latest     \
		-f _build/Dockerfile.akashctl \
		_build
	docker build --rm             \
		-t ovrclk/akashd:latest     \
		-f _build/Dockerfile.akashd \
		_build

install:
	$(GO) install $(BUILD_FLAGS) ./cmd/akashctl
	$(GO) install $(BUILD_FLAGS) ./cmd/akashd

image-minikube:
	eval $$(minikube docker-env) && make image

shellcheck:
	docker run --rm \
	--volume ${PWD}:/shellcheck \
	--entrypoint sh \
	koalaman/shellcheck-alpine:stable \
	-x /shellcheck/script/shellcheck.sh

test:
	$(GO) test -tags=$(BUILD_MAINNET) ./...

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
	golangci-lint run

lintdeps-install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

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

LINT = $(GOBIN)/golangci-lint run ./... --disable-all --enable 

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

deps-install:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

devdeps-install: lintdeps-install kubetypes-deps-install
	$(GO) install github.com/vektra/mockery/.../
	$(GO) install k8s.io/code-generator/...
	$(GO) install sigs.k8s.io/kind
	$(GO) install golang.org/x/tools/cmd/stringer

test-integration: $(BINS)
	cp akashctl akashd ./_build
	go test -mod=readonly -p 4 -tags "integration $(BUILD_MAINNET)" -v ./integration/...

test-e2e-integration: $(BINS)
	# ASSUMES:
	# 1. cluster created - `kind create cluster --config=_run/kube/kind-config.yaml`
	# 2. cluster setup - `make -s -C _run/kube kind-ingress-setup`
	cp akashctl akashd ./_build
	$(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestE2EApp

test-query-app: $(BINS)
	 $(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestQueryApp

test-k8s-integration:
	# ASSUMES:
	# 1. cluster created - `kind create cluster`
	# 2. cluster setup   - ./script/setup-kind.sh
	go test -v -tags k8s_integration ./pkg/apis/akash.network/v1
	go test -v -tags k8s_integration ./provider/cluster/kube

gentypes: $(PROTOC_FILES)

vendor:
	go mod vendor

kubetypes-deps-install:
	if [ -d "$(shell go env GOPATH)/src/k8s.io/code-generator" ]; then    \
		cd "$(shell go env GOPATH)/src/k8s.io/code-generator" && git pull;  \
		exit 0;                                                             \
	fi;                                                                   \
	mkdir -p "$(shell go env GOPATH)/src/k8s.io" && \
	git clone                                       \
	  git@github.com:kubernetes/code-generator.git  \
		"$(shell go env GOPATH)/src/k8s.io/code-generator"

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

clean:
	rm -f $(BINS) $(IMAGE_BINS)

.PHONY: all bins build \
	akashctl akashd \
	image image-bins \
	test test-nocache test-full test-coverage \
	deps-install devdeps-install \
	test-integraion \
	test-lint lintdeps-install \
	test-k8s-integration \
	test-vet \
	vendor \
	mocks \
	gofmt \
	docs \
	clean \
	kubetypes kubetypes-deps-install \
	install

update-swagger-docs:
	statik -src=cmd/swagger-ui -dest=cmd -f -m
	@if [ -n "$(git status --porcelain)" ]; then \
        echo "\033[91mSwagger docs are out of sync!!!\033[0m";\
        exit 1;\
    else \
    	echo "\033[92mSwagger docs are in sync\033[0m";\
    fi

###############################################################################
###                           Simulation                                    ###
###############################################################################

test-sim-fullapp:
	@echo "Running app simulation test..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) ${APP_DIR} -run=TestFullAppSimulation -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-nondeterminism:
	@echo "Running non-determinism test. This may take several minutes..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run TestAppStateDeterminism -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Period=0 -v -timeout 24h

test-sim-import-export:
	@echo "Running application import/export simulation..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run=TestAppImportExport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-after-import:
	@echo "Running application simulation-after-import..."
	go test -mod=readonly -tags=$(BUILD_MAINNET) $(APP_DIR) -run=TestAppSimulationAfterImport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sims: test-sim-fullapp test-sim-nondeterminism test-sim-import-export test-sim-after-import

.PHONY: modvendor
modvendor: modsensure $(MODVENDOR)
	@echo "vendoring non-go files files..."
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=github.com/zondax/hid

# Tools installation
$(CACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(CACHE_BIN)
	mkdir -p $(CACHE_INCLUDE)
	mkdir -p $(CACHE_VERSIONS)

$(MODVENDOR): $(CACHE)
	@echo "installing modvendor..."
	GOBIN=$(CACHE_BIN) GO111MODULE=off go get github.com/goware/modvendor

cache-clean:
	rm -rf $(CACHE)

.PHONY: modsensure
modsensure: deps-tidy

.PHONY: codegen
codegen: generate kubetypes

.PHONY: setup-devenv
setup-devenv: $(GOLANGCI_LINT) $(BUF) $(PROTOC) $(MODVENDOR) modvendor

.PHONY: setup-cienv
setup-cienv: modvendor $(GOLANGCI_LINT)

.PHONY: release-dry-run
release-dry-run: modvendor
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		goreng/golang-cross:$(GOLANG_CROSS_VERSION) \
		--rm-dist --skip-validate --skip-publish

.PHONY: release
release: modvendor
	@if [ -z "$(DOCKER_USERNAME)" ]; then \
		echo "\033[91mDOCKER_USERNAME is required for release\033[0m";\
		exit 1;\
	fi
	@if [ -z "$(DOCKER_PASSWORD)" ]; then \
		echo "\033[91mDOCKER_PASSWORD is required for release\033[0m";\
		exit 1;\
	fi
	@if [ -z "$(GORELEASER_ACCESS_TOKEN)" ]; then \
		echo "\033[91mGORELEASER_ACCESS_TOKEN is required for release\033[0m";\
		exit 1;\
	fi
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e DOCKER_USERNAME=$(DOCKER_USERNAME) \
		-e DOCKER_PASSWORD=$(DOCKER_PASSWORD) \
		-e GITHUB_TOKEN=$(GORELEASER_ACCESS_TOKEN) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		goreng/golang-cross:${GOLANG_CROSS_VERSION} \
		release --rm-dist
