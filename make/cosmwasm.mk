.PHONY: build-contracts
build-contracts:
	mkdir -p $(AKASH_DEVCACHE)/cosmwasm
	docker run --rm \
	  --platform linux/amd64 \
	  -v "$(ROOT_DIR)":/code \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/target":/target \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/artifacts":/code/artifacts \
	  --mount type=volume,source=registry_cache,target=/usr/local/cargo/registry \
	  $(COSMWASM_OPTIMIZER_IMAGE)
	@# Cargo normalizes hyphenated crate names to underscores. Keep the
	@# released artifact names aligned with the contract directories.
	@artifacts="$(AKASH_DEVCACHE)/cosmwasm/artifacts"; \
	for artifact in "pyth_vaa.wasm:pyth-vaa.wasm" "pyth_pro.wasm:pyth-pro.wasm"; do \
		old="$${artifact%%:*}"; \
		new="$${artifact##*:}"; \
		if [ ! -f "$$artifacts/$$old" ]; then \
			echo "error: expected $$artifacts/$$old from CosmWasm optimizer" >&2; \
			exit 1; \
		fi; \
		mv -f "$$artifacts/$$old" "$$artifacts/$$new"; \
	done; \
	if [ -f "$$artifacts/checksums.txt" ]; then \
		tmp="$$artifacts/checksums.txt.tmp"; \
		sed \
			-e 's/pyth_vaa\.wasm/pyth-vaa.wasm/g' \
			-e 's/pyth_pro\.wasm/pyth-pro.wasm/g' \
			"$$artifacts/checksums.txt" > "$$tmp"; \
		mv "$$tmp" "$$artifacts/checksums.txt"; \
	fi

.PHONY: generate-contracts
generate-contracts: build-contracts
	$(ROOT_DIR)/script/wasm2go.sh
