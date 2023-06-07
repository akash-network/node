COVER_PACKAGES = $(shell go list ./... | grep -v mock | paste -sd, -)

TEST_MODULE ?= ./...

###############################################################################
###                           Misc tests                                    ###
###############################################################################

.PHONY: shellcheck
shellcheck:
	docker run --rm \
	--volume ${PWD}:/shellcheck \
	--entrypoint sh \
	koalaman/shellcheck-alpine:stable \
	-x /shellcheck/script/shellcheck.sh

.PHONY: test
test:
	$(GO_TEST) -v -timeout 300s $(TEST_MODULE)

.PHONY: test-nocache
test-nocache:
	$(GO_TEST) -count=1 $(TEST_MODULE)

.PHONY: test-full
test-full:
	$(GO_TEST) -v -tags=$(BUILD_TAGS) $(TEST_MODULE)

.PHONY: test-coverage
test-coverage:
	$(GO_TEST) -tags=$(BUILD_MAINNET) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="$(COVER_PACKAGES)" \
		./...

.PHONY: test-vet
test-vet:
	$(GO_VET) ./...
