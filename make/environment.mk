.PHONY: setup-devenv
setup-devenv: $(GOLANGCI_LINT) $(PROTOC) $(GRPC_GATEWAY) $(MODVENDOR) protoc-swagger deps-vendor modvendor

.PHONY: setup-cienv
setup-cienv: deps-vendor modvendor $(GOLANGCI_LINT)
