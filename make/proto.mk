###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
	GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-linux-x86_64
endif
ifeq ($(UNAME_OS),Darwin)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
	GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-darwin-x86_64
endif

.PHONY: proto-lint
proto-lint:
	$(DOCKER_BUF) check lint --error-format=json

.PHONY: proto-check-breaking
proto-check-breaking:
	rm -rf $(CACHE)/akash
	mkdir -p $(CACHE)/akash
	(cp -r .git $(CACHE)/akash; \
	cd $(CACHE)/akash; \
	git checkout master; \
	git reset --hard; \
	$(MAKE) modvendor)
	$(DOCKER_BUF) check breaking --against-input '.cache/akash/'
	rm -rf $(CACHE)/akash

.PHONY: proto-format
proto-format:
	$(DOCKER_CLANG) find ./ ! -path "./vendor/*" -name *.proto -exec clang-format -i {} \;
