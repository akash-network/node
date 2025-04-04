SUBLINTERS = unused \
			misspell \
			gofmt \
			gocritic \
			goconst \
			ineffassign \
			unparam \
			staticcheck \
			copyloopvar \
			prealloc
# TODO: ^ gochecknoglobals

# Execute the same lint methods as configured in .github/workflows/tests.yaml
# Clear feedback from each method as it fails.
.PHONY: test-sublinters
test-sublinters: $(patsubst %, test-sublinter-%,$(SUBLINTERS))

.PHONY: test-lint-all
test-lint-all: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT_RUN) ./... --issues-exit-code=0 --timeout=10m

.PHONY: test-sublinter-misspell
test-sublinter-misspell: $(GOLANGCI_LINT)
	$(LINT) misspell --no-config

.PHONY: test-sublinter-ineffassign
test-sublinter-ineffassign: $(GOLANGCI_LINT)
	$(LINT) ineffassign --no-config

.PHONY: test-sublinter-%
test-sublinter-%: $(GOLANGCI_LINT)
	$(LINT) $*
