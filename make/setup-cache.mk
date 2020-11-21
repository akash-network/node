$(CACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(CACHE_BIN)
	mkdir -p $(CACHE_INCLUDE)
	mkdir -p $(CACHE_VERSIONS)
	mkdir -p $(CACHE_NODE_MODULES)

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

$(PROTOC_GEN_COSMOS_VERSION_FILE): $(CACHE)
	@echo "installing protoc-gen-cosmos..."
	rm -f $(PROTOC_GEN_COSMOS)
	GOBIN=$(CACHE_BIN) $(GO) install github.com/regen-network/cosmos-proto/protoc-gen-gocosmos
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(PROTOC_GEN_COSMOS): $(PROTOC_GEN_COSMOS_VERSION_FILE)

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

$(STATIK_VERSION_FILE): $(CACHE)
	@echo "Installing statik..."
	rm -f $(STATIK)
	GOBIN=$(CACHE_BIN) $(GO) get github.com/rakyll/statik@$(STATIK_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(STATIK): $(STATIK_VERSION_FILE)

$(MODVENDOR): $(CACHE)
	@echo "installing modvendor..."
	GOBIN=$(CACHE_BIN) GO111MODULE=off go get github.com/goware/modvendor

$(SWAGGER_COMBINE): $(CACHE)
ifeq (, $(shell which swagger-combine 2>/dev/null))
	@echo "Installing swagger-combine..."
	npm install swagger-combine --prefix $(CACHE_NODE_MODULES)
else
	@echo "swagger-combine already installed; skipping..."
endif

$(PROTOC_SWAGGER_GEN): $(CACHE)
ifeq (, $(shell which protoc-gen-swagger 2>/dev/null))
	@echo "installing protoc-gen-swagger..."
	GOBIN=$(CACHE_BIN) $(GO) get github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger
endif

kubetypes-deps-install:
	if [ -d "$(shell go env GOPATH)/src/k8s.io/code-generator" ]; then    \
		cd "$(shell go env GOPATH)/src/k8s.io/code-generator" && git pull;  \
		exit 0;                                                             \
	fi;                                                                   \
	mkdir -p "$(shell go env GOPATH)/src/k8s.io" && \
	git clone https://github.com/kubernetes/code-generator.git \
		"$(shell go env GOPATH)/src/k8s.io/code-generator"

devdeps-install: kubetypes-deps-install
	$(GO) install github.com/vektra/mockery/.../
	$(GO) install k8s.io/code-generator/...
	$(GO) install sigs.k8s.io/kind
	$(GO) install golang.org/x/tools/cmd/stringer

cache-clean:
	rm -rf $(CACHE)
