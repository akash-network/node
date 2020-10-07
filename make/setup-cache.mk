$(CACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(CACHE_BIN)
	mkdir -p $(CACHE_INCLUDE)
	mkdir -p $(CACHE_VERSIONS)

$(GOLANGCI_LINT_VERSION_FILE): $(CACHE)
	@echo "installing golangci-lint..."
	rm -f $(GOLANGCI_LINT)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(CACHE_BIN) $(GOLANGCI_LINT_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GOLANGCI_LINT): $(GOLANGCI_LINT_VERSION_FILE)

.PHONY:lintdeps-install
lintdeps-install: $(GOLANGCI_LINT)
	@echo "lintdeps-install is deprecated and will be removed once Github Actions migrated to use .cache/bin/golangci-lint"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
		sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION)

$(BUF_VERSION_FILE): $(CACHE)
	@echo "installing protoc buf cli..."
	rm -f $(BUF)
	curl -sSL \
		"https://github.com/bufbuild/buf/releases/download/v$(BUF_VERSION)/buf-$(UNAME_OS)-$(UNAME_ARCH)" \
		-o "$(CACHE_BIN)/buf"
	chmod +x "$(CACHE_BIN)/buf"
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(BUF): $(BUF_VERSION_FILE)

$(PROTOC_VERSION_FILE): $(CACHE)
	@echo "installing protoc compiler..."
	rm -f $(PROTOC)
	(cd /tmp; \
	curl -sOL "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) bin/protoc; \
	unzip -oq ${PROTOC_ZIP} -d $(CACHE) 'include/*'; \
	rm -f ${PROTOC_ZIP})
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(PROTOC): $(PROTOC_VERSION_FILE)

$(GRPC_GATEWAY_VERSION_FILE): $(CACHE)
	@echo "Installing protoc-gen-grpc-gateway..."
	rm -f $(GRPC_GATEWAY)
	curl -o "${CACHE_BIN}/protoc-gen-grpc-gateway" -L \
	"https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v${GRPC_GATEWAY_VERSION}/${GRPC_GATEWAY_BIN}"
	chmod +x "$(CACHE_BIN)/protoc-gen-grpc-gateway"
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GRPC_GATEWAY): $(GRPC_GATEWAY_VERSION_FILE)

$(MODVENDOR): $(CACHE)
	@echo "installing modvendor..."
	GOBIN=$(CACHE_BIN) GO111MODULE=off go get github.com/goware/modvendor

protoc-swagger:
ifeq (, $(shell which swagger-combine))
	@echo "Installing swagger-combine..."
	npm install -g swagger-combine
else
	@echo "swagger-combine already installed; skipping..."
endif

kubetypes-deps-install:
	if [ -d "$(shell go env GOPATH)/src/k8s.io/code-generator" ]; then    \
		cd "$(shell go env GOPATH)/src/k8s.io/code-generator" && git pull;  \
		exit 0;                                                             \
	fi;                                                                   \
	mkdir -p "$(shell go env GOPATH)/src/k8s.io" && \
	git clone  git@github.com:kubernetes/code-generator.git \
		"$(shell go env GOPATH)/src/k8s.io/code-generator"

devdeps-install: $(GOLANGCI_LINT) kubetypes-deps-install
	$(GO) install github.com/vektra/mockery/.../
	$(GO) install k8s.io/code-generator/...
	$(GO) install sigs.k8s.io/kind
	$(GO) install golang.org/x/tools/cmd/stringer
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger

cache-clean:
	rm -rf $(CACHE)
