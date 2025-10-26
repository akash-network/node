COVER_PACKAGES = $(shell go list ./... | grep -v mock | paste -sd, -)

TEST_MODULES ?= $(shell $(GO) list ./... | grep -v '/mocks')

###############################################################################
###                           Misc tests                                    ###
###############################################################################

.PHONY: test
test:
	$(GO_TEST) -v -timeout 600s $(TEST_MODULES)

.PHONY: test-nocache
test-nocache:
	$(GO_TEST) -count=1 $(TEST_MODULES)

.PHONY: test-full
test-full:
	$(GO_TEST) -v -tags=$(BUILD_TAGS) $(TEST_MODULES)

.PHONY: test-integration
test-integration:
	$(GO_TEST) -v -tags="e2e.integration" $(TEST_MODULES)

.PHONY: test-coverage
test-coverage:
	$(GO_TEST) -tags=$(BUILD_MAINNET) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="$(COVER_PACKAGES)" \
		./...

.PHONY: test-vet
test-vet:
	$(GO_VET) ./...
