###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
  GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-linux-x86_64
endif
ifeq ($(UNAME_OS),Darwin)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
  GRPC_GATEWAY_BIN ?= protoc-gen-grpc-gateway-v${GRPC_GATEWAY_VERSION}-darwin-x86_64
endif

proto-lint: $(BUF) modvendor
	$(BUF) check lint --error-format=json

proto-check-breaking: $(BUF) modvendor
	$(BUF) check breaking --against-input '.git#branch=master'
