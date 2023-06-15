$(AKASH_DEVCACHE):
	@echo "creating .cache dir structure..."
	mkdir -p $@
	mkdir -p $(AKASH_DEVCACHE_BIN)
	mkdir -p $(AKASH_DEVCACHE_INCLUDE)
	mkdir -p $(AKASH_DEVCACHE_VERSIONS)
	mkdir -p $(AKASH_DEVCACHE_NODE_MODULES)
	mkdir -p $(AKASH_DEVCACHE)/run
cache: $(AKASH_DEVCACHE)

$(GIT_CHGLOG_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "installing git-chglog $(GIT_CHGLOG_VERSION) ..."
	rm -f $(GIT_CHGLOG)
	GOBIN=$(AKASH_DEVCACHE_BIN) go install github.com/git-chglog/git-chglog/cmd/git-chglog@$(GIT_CHGLOG_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GIT_CHGLOG): $(GIT_CHGLOG_VERSION_FILE)

$(MOCKERY_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "installing mockery $(MOCKERY_VERSION) ..."
	rm -f $(MOCKERY)
	GOBIN=$(AKASH_DEVCACHE_BIN) go install -ldflags '-s -w -X github.com/vektra/mockery/v2/pkg/config.SemVer=$(MOCKERY_VERSION)' github.com/vektra/mockery/v2@v$(MOCKERY_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(MOCKERY): $(MOCKERY_VERSION_FILE)

$(GOLANGCI_LINT_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "installing golangci-lint $(GOLANGCI_LINT_VERSION) ..."
	rm -f $(GOLANGCI_LINT)
	GOBIN=$(AKASH_DEVCACHE_BIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(GOLANGCI_LINT): $(GOLANGCI_LINT_VERSION_FILE)

$(STATIK_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "Installing statik $(STATIK_VERSION) ..."
	rm -f $(STATIK)
	GOBIN=$(AKASH_DEVCACHE_BIN) $(GO) install github.com/rakyll/statik@$(STATIK_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(STATIK): $(STATIK_VERSION_FILE)

$(COSMOVISOR_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "installing cosmovisor $(COSMOVISOR_VERSION) ..."
	rm -f $(COSMOVISOR)
	GOBIN=$(AKASH_DEVCACHE_BIN) $(GO) install cosmossdk.io/tools/cosmovisor/cmd/cosmovisor@$(COSMOVISOR_VERSION)
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(COSMOVISOR): $(COSMOVISOR_VERSION_FILE)

cache-clean:
	rm -rf $(AKASH_DEVCACHE)
