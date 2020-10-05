###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
endif
ifeq ($(UNAME_OS),Darwin)
  PROTOC_ZIP ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
endif

proto-lint: $(BUF)
	$(BUF) check lint --error-format=json

proto-check-breaking: $(BUF)
	$(BUF) check breaking --against-input '.git#branch=master'
