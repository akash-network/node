.PHONY: setup-devenv
setup-devenv: $(PROTOC) $(GRPC_GATEWAY) $(MODVENDOR) protoc-swagger deps-vendor modvendor

.PHONY: setup-cienv
setup-cienv: deps-vendor modvendor
