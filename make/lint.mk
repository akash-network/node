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

