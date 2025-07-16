.PHONY: generate
generate: $(MOCKERY)
	$(GO) generate ./...

.PHONY: codegen
codegen: generate

mocks: $(MOCKERY)
	$(MOCKERY)
