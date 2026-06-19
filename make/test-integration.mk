COVER_PACKAGES = $(shell go list ./... | grep -v mock | paste -sd, -)

TEST_MODULES ?= $(shell $(GO) list ./... | grep -v '/mocks')

###############################################################################
###                           Misc tests                                    ###
###############################################################################

.PHONY: test
test: wasmvm-libs
	$(GO_TEST) $(BUILD_FLAGS) -v -timeout 600s $(TEST_MODULES)

.PHONY: test-nocache
test-nocache: wasmvm-libs
	$(GO_TEST) $(BUILD_FLAGS) -count=1 $(TEST_MODULES)

.PHONY: test-full
test-full: wasmvm-libs
	$(GO_TEST) -v $(BUILD_FLAGS) $(TEST_MODULES)

.PHONY: test-integration
test-integration:
	$(GO_TEST) -v -tags="e2e.integration" -ldflags '$(ldflags)' ./tests/e2e/...

# test-grpc-surface runs the exhaustive gRPC transaction/query suite against an
# in-process single-validator network (the fast, standalone-compilable mirror of
# the post-upgrade verification that runs against a testnetify-forked node).
.PHONY: test-grpc-surface
test-grpc-surface: wasmvm-libs
	$(GO_TEST) -v -tags="e2e.integration" -ldflags '$(ldflags)' -timeout 30m ./tests/fullsurface/... -args -grpc-suite-mode=all

.PHONY: test-grpc-surface-tx
test-grpc-surface-tx: wasmvm-libs
	$(GO_TEST) -v -tags="e2e.integration" -ldflags '$(ldflags)' -timeout 30m ./tests/fullsurface/... -args -grpc-suite-mode=tx

.PHONY: test-grpc-surface-query
test-grpc-surface-query: wasmvm-libs
	$(GO_TEST) -v -tags="e2e.integration" -ldflags '$(ldflags)' -timeout 10m ./tests/fullsurface/... -args -grpc-suite-mode=query

.PHONY: test-coverage
test-coverage: wasmvm-libs
	$(GO_TEST) $(BUILD_FLAGS) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="$(COVER_PACKAGES)" \
		./...

.PHONY: test-vet
test-vet: wasmvm-libs
	$(GO_VET) $(BUILD_FLAGS) ./...
