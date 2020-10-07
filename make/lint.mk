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

test-sublinter-misspell:
	$(LINT) misspell --no-config

test-sublinter-ineffassign:
	$(LINT) ineffassign --no-config

test-sublinter-%:
	$(LINT) "$(@:test-sublinter-%=%)"
