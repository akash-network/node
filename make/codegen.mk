.PHONY: generate
generate:
	$(GO) generate ./...

.PHONY: mocks
mocks: $(MOCKERY) modvendor
	$(MOCKERY) --case=underscore --dir vendor/github.com/cosmos/cosmos-sdk/x/bank/types --output testutil/cosmos_mock        --name QueryClient --outpkg cosmos_mocks --keeptree
	$(MOCKERY) --case=underscore --dir client/broadcaster                               --output client/broadcaster/mocks    --name Client
	$(MOCKERY) --case=underscore --dir client                                           --output client/mocks/               --name QueryClient
	$(MOCKERY) --case=underscore --dir client                                           --output client/mocks/               --name Client
	$(MOCKERY) --case=underscore --dir x/escrow/keeper                                  --output x/escrow/keeper/mocks       --name BankKeeper
	$(MOCKERY) --case=underscore --dir x/deployment/handler                             --output x/deployment/handler/mocks  --name AuthzKeeper

.PHONY: codegen
codegen: generate
