###############################################################################
###                           Protobuf                                    ###
###############################################################################
ifeq ($(UNAME_OS),Linux)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-linux-x86_64.zip
	CLANG_FORMAT_BIN ?= $(shell [ -f /etc/debian_version ] && echo "clang-format-6.0" || echo "clang-format")
endif
ifeq ($(UNAME_OS),Darwin)
	PROTOC_ZIP       ?= protoc-${PROTOC_VERSION}-osx-x86_64.zip
	# clang-format on macos is keg-only
	CLANG_FORMAT_BIN ?= /usr/local/opt/clang-format/bin/clang-format
endif

.PHONY: proto-lint
proto-lint: 
	$(DOCKER_BUF) check lint --error-format=json

.PHONY: proto-check-breaking
proto-check-breaking:
	$(DOCKER_BUF) check breaking --against-input '.git#branch=master'

.PHONY: proto-format
proto-format:
	$(DOCKER_CLANG) find ./ ! -path "./vendor/*" ! -path "./.cache/*" -name *.proto -exec clang-format -i {} \;
