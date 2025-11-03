.PHONY: build-contracts
build-contracts:
	mkdir -p $(AKASH_DEVCACHE)/cosmwasm
	docker run --rm \
	  -v "$(ROOT_DIR)":/code \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/target":/target \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/artifacts":/code/artifacts \
	  --mount type=volume,source=registry_cache,target=/usr/local/cargo/registry \
	  $(COSMWASM_OPTIMIZER_IMAGE)
