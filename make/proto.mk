###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
	CLANG_FORMAT_BIN ?= $(shell [ -f /etc/debian_version ] && echo "clang-format-6.0" || echo "clang-format")
	GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-linux-x86_64
endif
ifeq ($(UNAME_OS),Darwin)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
	# clang-format on macos is keg-only
	CLANG_FORMAT_BIN ?= /usr/local/opt/clang-format/bin/clang-format
	GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-darwin-x86_64
endif

.PHONY: proto-lint
proto-lint: $(BUF) modvendor
	$(BUF) check lint --error-format=json

.PHONY: proto-check-breaking
proto-check-breaking: $(BUF) modvendor
	$(BUF) check breaking --against-input $(HTTPS_GIT)#branch=master

.PHONY: proto-format
proto-format: clang-format-install
	find ./ ! -path "./vendor/*" ! -path "./.cache/*" -name *.proto -exec ${CLANG_FORMAT_BIN} -i {} \;
