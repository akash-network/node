COVER_PACKAGES = $(shell go list ./... | grep -v mock | paste -sd, -)

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
	$(GO) test -tags=$(BUILD_MAINNET) -timeout 300s ./...

.PHONY: test-nocache
test-nocache:
	$(GO) test -tags=$(BUILD_MAINNET) -count=1 ./...

.PHONY: test-full
test-full:
	$(GO) test -tags=$(BUILD_TAGS) -race ./...

.PHONY: test-coverage
test-coverage:
	$(GO) test -tags=$(BUILD_MAINNET) -coverprofile=coverage.txt \
		-covermode=count \
		-coverpkg="$(COVER_PACKAGES)" \
		./...

.PHONY: test-vet
test-vet:
	$(GO) vet ./...
