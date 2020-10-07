.PHONY: setup-devenv
setup-devenv: $(GOLANGCI_LINT) $(BUF) $(PROTOC) $(MODVENDOR) deps-vendor modvendor

.PHONY: setup-cienv
setup-cienv: deps-vendor modvendor $(GOLANGCI_LINT)
