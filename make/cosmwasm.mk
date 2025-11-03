.PHONY: build-contract-%
build-contract-%:
	mkdir -p $(AKASH_DEVCACHE)/cosmwasm/$*
	docker run --rm -v "$(ROOT_DIR)/contracts/$*":/code \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/$*":/target \
	  --mount type=volume,source=registry_cache,target=/usr/local/cargo/registry \
	  $(COSMWASM_OPTIMIZER_IMAGE)

.PHONY: build-contracts
build-contracts: build-contract-price-oracle
