COVER_PACKAGES = $(shell go list ./... | grep -v mock)
###############################################################################
###                           Integration                                   ###
###############################################################################

test-e2e-integration:
	# Assumes cluster created: `make -s -C _run/kube kind-cluster-create`
	$(KIND_VARS) go test -count=1 -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestIntegrationTestSuite

test-e2e-integration-k8s:
	KUBE_INGRESS_IP=127.0.0.1 KUBE_INGRESS_PORT=10080 go test -count=1 -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestIntegrationTestSuite

test-query-app:
	 $(KIND_VARS) go test -mod=readonly -p 4 -tags "e2e integration $(BUILD_MAINNET)" -v ./integration/... -run TestQueryApp

test-k8s-integration:
	# ASSUMES:
	# 1. cluster created - `kind create cluster`
	# 2. cluster setup   - ./script/setup-kind.sh
	go test -v -tags k8s_integration ./pkg/apis/akash.network/v1
	go test -v -tags k8s_integration ./provider/cluster/kube


###############################################################################
###                           Misc tests                                    ###
###############################################################################

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
		-coverpkg=$(COVER_PACKAGES)

test-vet:
	$(GO) vet ./...
