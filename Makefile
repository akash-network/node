BINS       := akashctl akashd
IMAGE_BINS := _build/akashctl _build/akashd
APP_DIR := ./app

GO := GO111MODULE=on go

GOLANGCI_LINT_VERSION = v1.27.0

IMAGE_BUILD_ENV = GOOS=linux GOARCH=amd64

BUILD_FLAGS = -mod=readonly -tags "netgo ledger" -ldflags \
 '-X github.com/cosmos/cosmos-sdk/version.Name=akash \
  -X github.com/cosmos/cosmos-sdk/version.ServerName=akashd \
  -X github.com/cosmos/cosmos-sdk/version.ClientName=akashctl \
  -X "github.com/cosmos/cosmos-sdk/version.BuildTags=netgo,ledger" \
  -X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags | sed 's/^v//') \
  -X github.com/cosmos/cosmos-sdk/version.Commit=$(shell git log -1 --format='%H')'

all: build bins

bins: $(BINS)

build:
	$(GO) build ./...

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

release:
	docker run --rm --privileged \
	-v $(PWD):/go/src/github.com/ovrclk/akash \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-w /go/src/github.com/ovrclk/akash \
	-e GITHUB_TOKEN \
	-e DOCKER_USERNAME \
	-e DOCKER_PASSWORD \
	-e DOCKER_REGISTRY \
	goreleaser/goreleaser release --rm-dist

image-minikube:
	eval $$(minikube docker-env) && make image

test:
	$(GO) test ./...

test-nocache:
	$(GO) test -count=1 ./...

test-full:
	$(GO) test -race ./...

test-coverage:
	$(GO) test -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="./..." \
		./...

test-lint:
	golangci-lint run

lintdeps-install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

test-vet:
	$(GO) vet ./...

deps-install:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

devdeps-install:
	$(GO) install github.com/vektra/mockery/.../

test-integration: $(BINS)
	cp akashctl akashd ./_build
	go test -mod=readonly -p 4 -tags=integration -v ./integration/...

gentypes: $(PROTOC_FILES)

vendor:
	go mod vendor

kubetypes:
	chmod +x vendor/k8s.io/code-generator/generate-groups.sh
	vendor/k8s.io/code-generator/generate-groups.sh all \
  	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
  	akash.network:v1

mocks:
	mockery -case=underscore -dir query                 -output query/mocks                 -name Client
	mockery -case=underscore -dir txutil                -output txutil/mocks                -name Client
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Client
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Engine
	mockery -case=underscore -dir app/market            -output app/market/mocks            -name Facilitator
	mockery -case=underscore -dir marketplace           -output marketplace/mocks           -name Handler
	mockery -case=underscore -dir provider              -output provider/mocks              -name StatusClient
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Cluster
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Deployment
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Reservation
	mockery -case=underscore -dir provider/cluster/kube -output provider/cluster/kube/mocks -name Client
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name Handler


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
	test-vet \
	vendor \
	mocks \
	gofmt \
	docs \
	clean \
	kubetypes \
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
	go test -mod=readonly ${APP_DIR} -run=TestFullAppSimulation -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-nondeterminism:
	@echo "Running non-determinism test. This may take several minutes..."
	go test -mod=readonly $(APP_DIR) -run TestAppStateDeterminism -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Period=0 -v -timeout 24h

test-sim-import-export:
	@echo "Running application import/export simulation..."
	go test -mod=readonly $(APP_DIR) -run=TestAppImportExport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m

test-sim-after-import:
	@echo "Running application simulation-after-import..."
	go test -mod=readonly $(APP_DIR) -run=TestAppSimulationAfterImport -Enabled=true \
		-NumBlocks=50 -BlockSize=100 -Commit=true -Seed=99 -Period=5 -v -timeout 10m
