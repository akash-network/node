KEY_NAME    ?= main
KEY_ADDRESS ?= $(shell $(AKASH) $(KEY_OPTS) keys show "$(KEY_NAME)" -a)

SDL_PATH ?= deployment.yaml

DSEQ           ?= 1
GSEQ           ?= 1
OSEQ           ?= 1
PRICE          ?= 10uakt
CERT_HOSTNAME  ?= localhost
LEASE_SERVICES ?= web

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
	$(AKASH) $(CHAIN_OPTS) tx broadcast "$(AKASH_HOME)/multisig-final.json"

.PHONY: provider-create
provider-create:
	$(AKASH) tx provider create "$(PROVIDER_CONFIG_PATH)" --from "$(PROVIDER_KEY_NAME)"

.PHONY: provider-update
provider-update:
	$(AKASH) tx provider update "$(PROVIDER_CONFIG_PATH)" --from "$(PROVIDER_KEY_NAME)"

.PHONY: provider-status
provider-status:
	$(PROVIDER_SERVICES) status $(PROVIDER_ADDRESS)

.PHONY: authenticate
authenticate:
	$(PROVIDER_SERVICES) authenticate \
		--from      "$(KEY_ADDRESS)" \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: auth-server
auth-server:
	$(PROVIDER_SERVICES) auth-server \
		--from "$(PROVIDER_KEY_NAME)" \
		--jwt-auth-listen-address "$(JWT_AUTH_HOST)" \

.PHONY: run-resource-server
run-resource-server:
	$(PROVIDER_SERVICES) run-resource-server \
		--from "$(PROVIDER_KEY_NAME)" \
		--resource-server-listen-address "$(RESOURCE_SERVER_HOST)" \
		--loki-gateway-listen-address localhost:3100 \

.PHONY: send-manifest
send-manifest:
	$(PROVIDER_SERVICES) send-manifest "$(SDL_PATH)" \
		--dseq "$(DSEQ)"     \
		--from "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)" \
		--auth-type "$(GW_AUTH_TYPE)"

.PHONY: get-manifest
get-manifest:
	$(PROVIDER_SERVICES) get-manifest \
		--dseq "$(DSEQ)"     \
		--from "$(KEY_NAME)" \
		--provide   "$(PROVIDER_ADDRESS)" \
		--auth-type "$(GW_AUTH_TYPE)"


.PHONY: deployment-create
deployment-create:
	$(AKASH) tx deployment create "$(SDL_PATH)" \
		--dseq "$(DSEQ)" \
		--from "$(KEY_NAME)"

.PHONY: deployment-deposit
deployment-deposit:
	$(AKASH) tx escrow deposit deployment "$(PRICE)" \
		--dseq "$(DSEQ)" \
		--from "$(KEY_NAME)"

.PHONY: deployment-update
deployment-update:
	$(AKASH) tx deployment update "$(SDL_PATH)" \
		--dseq "$(DSEQ)" \
		--from "$(KEY_NAME)"

.PHONY: deployment-close
deployment-close:
	$(AKASH) tx deployment close \
		--owner "$(MAIN_ADDR)" \
		--dseq "$(DSEQ)"       \
		--from "$(KEY_NAME)"

.PHONY: group-close
group-close:
	$(AKASH) tx deployment group close \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: group-pause
group-pause:
	$(AKASH) tx deployment group pause \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: group-start
group-start:
	$(AKASH) tx deployment group start \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: bid-create
bid-create:
	$(AKASH) tx market bid create \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--oseq  "$(OSEQ)"              \
		--from  "$(PROVIDER_KEY_NAME)" \
		--price "$(PRICE)"

.PHONY: bid-close
bid-close:
	$(AKASH) tx market bid close \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--oseq  "$(OSEQ)"              \
		--from  "$(PROVIDER_KEY_NAME)"

.PHONY: lease-create
lease-create:
	$(AKASH) tx market lease create \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--provider "$(PROVIDER_ADDRESS)" \
		--from  "$(KEY_NAME)"

.PHONY: lease-withdraw
lease-withdraw:
	$(AKASH) tx market lease withdraw \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--provider "$(PROVIDER_ADDRESS)" \
		--from  "$(PROVIDER_KEY_NAME)"

.PHONY: lease-close
lease-close:
	$(AKASH) tx market lease close \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--provider "$(PROVIDER_ADDRESS)" \
		--from  "$(KEY_NAME)"

.PHONY: query-accounts
query-accounts: $(patsubst %, query-account-%,$(GENESIS_ACCOUNTS))

.PHONY: query-account-%
query-account-%:
	$(AKASH) query bank balances "$(shell $(AKASH) $(KEY_OPTS) keys show -a "$(@:query-account-%=%)")"
	$(AKASH) query account       "$(shell $(AKASH) $(KEY_OPTS) keys show -a "$(@:query-account-%=%)")"

.PHONY: query-provider
query-provider:
	$(AKASH) query provider get "$(PROVIDER_ADDRESS)"

.PHONY: query-providers
query-providers:
	$(AKASH) query provider list

.PHONY: query-deployment
query-deployment:
	$(AKASH) query deployment get \
		--owner "$(KEY_ADDRESS)" \
		--dseq  "$(DSEQ)"

.PHONY: query-deployments
query-deployments:
	$(AKASH) query deployment list

.PHONY: query-order
query-order:
	$(AKASH) query market order get \
		--owner "$(KEY_ADDRESS)" \
		--dseq  "$(DSEQ)"        \
		--gseq  "$(GSEQ)"        \
		--oseq  "$(OSEQ)"

.PHONY: query-orders
query-orders:
	$(AKASH) query market order list

.PHONY: query-bid
query-bid:
	$(AKASH) query market bid get \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: query-bids
query-bids:
	$(AKASH) query market bid list

.PHONY: query-lease
query-lease:
	$(AKASH) query market lease get \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: query-leases
query-leases:
	$(AKASH) query market lease list

.PHONY: akash-node-ready
akash-node-ready: SHELL=$(BASH_PATH)
akash-node-ready:
	@( \
		max_retry=15; \
		counter=0; \
		while [[ $$counter -lt $$max_retry ]]; do \
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
