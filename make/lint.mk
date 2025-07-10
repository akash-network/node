SUBLINTERS = unused \
			misspell \
			gofmt \
			gocritic \
			goconst \
			ineffassign \
			unparam \
			staticcheck \
			revive \
			gosec \
			exportloopref \
			prealloc
# TODO: ^ gochecknoglobals

.PHONY: lint-go
lint-go: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT_RUN) ./... --issues-exit-code=0 --timeout=10m

# Execute the same lint methods as configured in .github/workflows/tests.yaml
# Clear feedback from each method as it fails.
.PHONY: test-sublinters
test-sublinters: $(patsubst %, test-sublinter-%,$(SUBLINTERS))

.PHONY: test-sublinter-%
test-sublinter-%: $(GOLANGCI_LINT)
# Execute the same lint methods as configured in .github/workflows/tests.yaml
# Clear feedback from each method as it fails.
.PHONY: test-sublinters
test-sublinters: $(patsubst %, test-sublinter-%,$(SUBLINTERS))

.PHONY: test-sublinter-misspell
test-sublinter-misspell: $(GOLANGCI_LINT)
	$(LINT) misspell --no-config

.PHONY: test-sublinter-ineffassign
test-sublinter-ineffassign: $(GOLANGCI_LINT)
	$(LINT) ineffassign --no-config

.PHONY: test-sublinter-%
test-sublinter-%: $(GOLANGCI_LINT)

.PHONY: lint-go-%
lint-go-%: $(GOLANGCI_LINT)
	$(LINT) $*

.PHONY: lint-shell
lint-shell:
	docker run --rm \
	--volume ${PWD}:/shellcheck \
	--entrypoint sh \
	koalaman/shellcheck-alpine:stable \
	-x /shellcheck/script/shellcheck.sh

