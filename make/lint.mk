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

.PHONY: test-sublinter-misspell
test-sublinter-misspell:
	$(LINT) misspell --no-config

.PHONY: test-sublinter-ineffassign
test-sublinter-ineffassign:
	$(LINT) ineffassign --no-config

.PHONY: test-sublinter-%
test-sublinter-%:
	$(LINT) $*
