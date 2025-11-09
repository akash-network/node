KEY_NAME ?= main

.PHONY: multisig-send
multisig-send:
	$(AKASH) tx send \
		"$(shell $(AKASH) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		"$(shell $(AKASH) $(KEY_OPTS) keys show "$(KEY_NAME)"     -a)" \
		1000000uakt \
		--generate-only \
		> "$(AKASH_HOME)/multisig-tx.json"
	$(AKASH) tx sign \
		"$(AKASH_HOME)/multisig-tx.json" \
		--multisig "$(shell $(AKASH) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		--from "main" \
		> "$(AKASH_HOME)/multisig-sig-main.json"
	$(AKASH) tx sign \
		"$(AKASH_HOME)/multisig-tx.json" \
		--multisig "$(shell $(AKASH) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		--from "other" \
		> "$(AKASH_HOME)/multisig-sig-other.json"
	$(AKASH) tx multisign \
		"$(AKASH_HOME)/multisig-tx.json" \
		"$(MULTISIG_KEY)" \
		"$(AKASH_HOME)/multisig-sig-main.json" \
		"$(AKASH_HOME)/multisig-sig-other.json" \
		> "$(AKASH_HOME)/multisig-final.json"
	$(AKASH) "$(CHAIN_OPTS)" tx broadcast "$(AKASH_HOME)/multisig-final.json"

.PHONY: akash-node-ready
akash-node-ready: SHELL=$(BASH_PATH)
akash-node-ready:
	@( \
		max_retry=15; \
		counter=0; \
		while [[ counter -lt max_retry ]]; do \
			read block < <(curl -s $(AKASH_NODE)/status | jq -r '.result.sync_info.latest_block_height' 2> /dev/null); \
			if [[ $$? -ne 0 || $$block -lt 1 ]]; then \
				echo "unable to get node status. sleep for 1s"; \
				((counter++)); \
				sleep 1; \
			else \
				echo "latest block height: $${block}"; \
				exit 0; \
			fi \
		done; \
		exit 1 \
	)
