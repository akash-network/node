.PHONY: build-contracts
build-contracts:
	mkdir -p $(AKASH_DEVCACHE)/cosmwasm
	docker run --rm \
	  --platform linux/amd64 \
	  -v "$(ROOT_DIR)":/code \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/target":/target \
	  -v "$(AKASH_DEVCACHE)/cosmwasm/artifacts":/code/artifacts \
	  --mount type=volume,source=registry_cache,target=/usr/local/cargo/registry \
	  --entrypoint /bin/sh \
	  $(COSMWASM_OPTIMIZER_IMAGE) \
	  -c 'mkdir -p artifacts && \
		rm -f artifacts/pyth-vaa.wasm artifacts/pyth-pro.wasm \
			artifacts/pyth_vaa.wasm artifacts/pyth_pro.wasm && \
		optimize.sh . && \
		cd artifacts && \
		for artifact in "pyth_vaa.wasm:pyth-vaa.wasm" "pyth_pro.wasm:pyth-pro.wasm"; do \
			old="$${artifact%%:*}"; \
			new="$${artifact##*:}"; \
			if [ ! -f "$$old" ]; then \
				echo "error: expected artifacts/$$old from CosmWasm optimizer" >&2; \
				exit 1; \
			fi; \
			mv -f "$$old" "$$new"; \
		done && \
		if [ -f checksums.txt ]; then \
			sed \
				-e "s/pyth_vaa\.wasm/pyth-vaa.wasm/g" \
				-e "s/pyth_pro\.wasm/pyth-pro.wasm/g" \
				checksums.txt > checksums.txt.tmp && \
			mv checksums.txt.tmp checksums.txt; \
		fi'

.PHONY: generate-contracts
generate-contracts: build-contracts
	$(ROOT_DIR)/script/wasm2go.sh
