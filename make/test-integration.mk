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
	$(GO_TEST) -v -tags="e2e.integration" -ldflags '$(ldflags)' $(TEST_MODULES)

.PHONY: test-coverage
test-coverage: wasmvm-libs
	$(GO_TEST) $(BUILD_FLAGS) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="$(COVER_PACKAGES)" \
		./...

.PHONY: test-vet
test-vet: wasmvm-libs
	$(GO_VET) $(BUILD_FLAGS) ./...
