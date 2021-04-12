KEY_NAME   ?= main


KEY_ADDRESS          ?= $(shell $(AKASHCTL_NONODE) $(KEY_OPTS) keys show "$(KEY_NAME)" -a)
PROVIDER_KEY_NAME    ?= provider
PROVIDER_ADDRESS     ?= $(shell $(AKASHCTL_NONODE) $(KEY_OPTS) keys show "$(PROVIDER_KEY_NAME)" -a)
PROVIDER_CONFIG_PATH ?= provider.yaml

SDL_PATH ?= deployment.yaml

DSEQ           ?= 1
GSEQ           ?= 1
OSEQ           ?= 1
PRICE          ?= 10uakt
CERT_HOSTNAME  ?= localhost
LEASE_SERVICES ?= web

.PHONY: multisig-send
multisig-send:
	$(AKASHCTL) tx send \
		"$(shell $(AKASHCTL) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		"$(shell $(AKASHCTL) $(KEY_OPTS) keys show "$(KEY_NAME)"     -a)" \
		1000000uakt \
		--generate-only \
		> "$(DATA_ROOT)/multisig-tx.json"
	$(AKASHCTL) tx sign \
		"$(DATA_ROOT)/multisig-tx.json" \
		--multisig "$(shell $(AKASHCTL) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		--from "main" \
		> "$(DATA_ROOT)/multisig-sig-main.json"
	$(AKASHCTL) tx sign \
		"$(DATA_ROOT)/multisig-tx.json" \
		--multisig "$(shell $(AKASHCTL) $(KEY_OPTS) keys show "$(MULTISIG_KEY)" -a)" \
		--from "other" \
		> "$(DATA_ROOT)/multisig-sig-other.json"
	$(AKASHCTL) tx multisign \
		"$(DATA_ROOT)/multisig-tx.json" \
		"$(MULTISIG_KEY)" \
		"$(DATA_ROOT)/multisig-sig-main.json" \
		"$(DATA_ROOT)/multisig-sig-other.json" \
		> "$(DATA_ROOT)/multisig-final.json"
	$(AKASHCTL) "$(CHAIN_OPTS)" tx broadcast "$(DATA_ROOT)/multisig-final.json"

.PHONY: provider-create
provider-create:
	$(AKASHCTL) tx provider create "$(PROVIDER_CONFIG_PATH)" \
		--from "$(PROVIDER_KEY_NAME)"

.PHONY: provider-update
provider-update:
	$(AKASHCTL) tx provider update "$(PROVIDER_CONFIG_PATH)" \
		--from "$(PROVIDER_KEY_NAME)"

.PHONY: provider-status
provider-status:
	$(AKASHCTL) provider status $(PROVIDER_ADDRESS)

.PHONY: send-manifest
send-manifest:
	$(AKASHCTL) provider send-manifest "$(SDL_PATH)" \
		--dseq "$(DSEQ)"     \
		--from "$(KEY_NAME)" \
		--provider "$(PROVIDER_ADDRESS)"

.PHONY: deployment-create
deployment-create:
	$(AKASHCTL) tx deployment create "$(SDL_PATH)" \
		--dseq "$(DSEQ)" 			   \
		--from "$(KEY_NAME)"

.PHONY: deploy-create
deploy-create:
	$(AKASHCTL) deploy create "$(SDL_PATH)" \
		--dseq "$(DSEQ)" 			   \
		--from "$(KEY_NAME)"

.PHONY: deployment-deposit
deployment-deposit:
	$(AKASHCTL) tx deployment deposit "$(PRICE)" \
		--dseq "$(DSEQ)" 			   \
		--from "$(KEY_NAME)"

.PHONY: deployment-update
deployment-update:
	$(AKASHCTL) tx deployment update "$(SDL_PATH)" \
		--dseq "$(DSEQ)" \
		--from "$(KEY_NAME)"

.PHONY: deployment-close
deployment-close:
	$(AKASHCTL) tx deployment close \
		--owner "$(MAIN_ADDR)" \
		--dseq "$(DSEQ)"       \
		--from "$(KEY_NAME)"

.PHONY: group-close
group-close:
	$(AKASHCTL) tx deployment group close \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: group-pause
group-pause:
	$(AKASHCTL) tx deployment group pause \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: group-start
group-start:
	$(AKASHCTL) tx deployment group start \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--from  "$(KEY_NAME)"

.PHONY: bid-create
bid-create:
	$(AKASHCTL) tx market bid create \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--oseq  "$(OSEQ)"              \
		--from  "$(PROVIDER_KEY_NAME)" \
		--price "$(PRICE)"

.PHONY: bid-close
bid-close:
	$(AKASHCTL) tx market bid close \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--oseq  "$(OSEQ)"              \
		--from  "$(PROVIDER_KEY_NAME)"

.PHONY: lease-create
lease-create:
	$(AKASHCTL) tx market lease create \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--provider "$(PROVIDER_ADDRESS)" \
		--from  "$(KEY_NAME)"

.PHONY: lease-withdraw
lease-withdraw:
	$(AKASHCTL) tx market lease withdraw \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--provider "$(PROVIDER_ADDRESS)" \
		--from  "$(PROVIDER_KEY_NAME)"

.PHONY: lease-close
lease-close:
	$(AKASHCTL) tx market lease close \
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
	$(AKASHCTL) query bank balances "$(shell $(AKASHCTL_NONODE) $(KEY_OPTS) keys show -a "$(@:query-account-%=%)")"
	$(AKASHCTL) query account       "$(shell $(AKASHCTL_NONODE) $(KEY_OPTS) keys show -a "$(@:query-account-%=%)")"

.PHONY: query-provider
query-provider:
	$(AKASHCTL) query provider get "$(PROVIDER_ADDRESS)"

.PHONY: query-providers
query-providers:
	$(AKASHCTL) query provider list

.PHONY: query-deployment
query-deployment:
	$(AKASHCTL) query deployment get \
		--owner "$(KEY_ADDRESS)" \
		--dseq  "$(DSEQ)"

.PHONY: query-deployments
query-deployments:
	$(AKASHCTL) query deployment list

.PHONY: query-order
query-order:
	$(AKASHCTL) query market order get \
		--owner "$(KEY_ADDRESS)" \
		--dseq  "$(DSEQ)"        \
		--gseq  "$(GSEQ)"        \
		--oseq  "$(OSEQ)"

.PHONY: query-orders
query-orders:
	$(AKASHCTL) query market order list

.PHONY: query-bid
query-bid:
	$(AKASHCTL) query market bid get \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: query-bids
query-bids:
	$(AKASHCTL) query market bid list

.PHONY: query-lease
query-lease:
	$(AKASHCTL) query market lease get \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: query-leases
query-leases:
	$(AKASHCTL) query market lease list

.PHONY: query-certificates
query-certificates:
	$(AKASHCTL) query cert list

.PHONY: query-account-certificates
query-account-certificates:
	$(AKASHCTL) query cert list --owner="$(KEY_ADDRESS)" --state="valid"

.PHONY: create-server-certificate
create-server-certificate:
	$(AKASHCTL) tx cert create server $(CERT_HOSTNAME) --from=$(KEY_NAME) --rie

.PHONY: revoke-certificate
revoke-certificate:
	$(AKASHCTL) tx cert revoke --from=$(KEY_NAME)

.PHONY: events-run
events-run:
	$(AKASHCTL) events

.PHONY: provider-lease-logs
provider-lease-logs:
	$(AKASHCTL) provider lease-logs \
		-f \
		--service="$(LEASE_SERVICES)" \
		--dseq "$(DSEQ)"     \
		--from "$(KEY_NAME)" \
		--provider "$(PROVIDER_ADDRESS)"

.PHONY: provider-lease-events
provider-lease-events:
	$(AKASHCTL) provider lease-events \
		-f \
		--dseq "$(DSEQ)"     \
		--from "$(KEY_NAME)" \
		--provider "$(PROVIDER_ADDRESS)"
