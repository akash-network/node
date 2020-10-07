###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
	CLANG_FORMAT_BIN ?= $(shell [ -f /etc/debian_version ] && echo "clang-format-6.0" || echo "clang-format")
endif
ifeq ($(UNAME_OS),Darwin)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
	CLANG_FORMAT_BIN ?= clang-format
endif

proto-lint: $(BUF)
	$(BUF) check lint --error-format=json

proto-check-breaking: $(BUF)
	$(BUF) check breaking --against-input '.git#branch=master'

proto-format: clang-format-install
	find ./ ! -path "./vendor/*" ! -path "./.cache/*" -name *.proto -exec ${CLANG_FORMAT_BIN} -i {} \;
