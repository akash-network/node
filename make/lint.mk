.PHONY: lint-go
lint-go: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./... --timeout=10m

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

