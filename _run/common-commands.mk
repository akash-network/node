KEY_NAME          ?= main
KEY_ADDRESS       ?= $(shell $(AKASHCTL_NONODE) keys show "$(KEY_NAME)" -a)

PROVIDER_KEY_NAME    ?= provider
PROVIDER_ADDRESS     ?= $(shell $(AKASHCTL_NONODE) keys show "$(PROVIDER_KEY_NAME)" -a)
PROVIDER_CONFIG_PATH ?= provider.yaml

SDL_PATH ?= deployment.yaml

DSEQ  ?= 1
GSEQ  ?= 1
OSEQ  ?= 1
PRICE ?= 10akash

.PHONY: provider-create
provider-create:
	$(AKASHCTL) tx provider create "$(PROVIDER_CONFIG_PATH)" -y \
		--from "$(PROVIDER_KEY_NAME)"

.PHONY: provider-update
provider-update:
	$(AKASHCTL) tx provider update "$(PROVIDER_CONFIG_PATH)" -y \
		--from "$(PROVIDER_KEY_NAME)"

.PHONY: deployment-create
deployment-create:
	$(AKASHCTL) tx deployment create "$(SDL_PATH)" -y \
		--dseq "$(DSEQ)" 			   \
		--from "$(KEY_NAME)"

.PHONY: deployment-close
deployment-close:
	$(AKASHCTL) tx deployment close -y \
		--owner "$(MAIN_ADDR)"   \
		--dseq "$(DSEQ)" 			   \
		--from "$(KEY_NAME)" -y

.PHONY: order-close
order-close:
	$(AKASHCTL) tx market order-close -y \
		--owner "$(KEY_ADDRESS)" \
		--dseq  "$(DSEQ)"        \
		--gseq  "$(GSEQ)"        \
		--oseq  "$(OSEQ)"        \
		--from  "$(KEY_NAME)"

.PHONY: bid-create
bid-create:
	$(AKASHCTL) tx market bid-create -y \
		--owner "$(KEY_ADDRESS)"       \
		--dseq  "$(DSEQ)"              \
		--gseq  "$(GSEQ)"              \
		--oseq  "$(OSEQ)"              \
		--from  "$(PROVIDER_KEY_NAME)" \
		--price "$(PRICE)"

.PHONY: bid-close
bid-close:
	$(AKASHCTL) tx market bid-close -y \
		--owner "$(KEY_ADDRESS)"         \
		--dseq  "$(DSEQ)"                \
		--gseq  "$(GSEQ)"                \
		--oseq  "$(OSEQ)"                \
		--from  "$(PROVIDER_KEY_NAME)"

.PHONY: query-accounts
query-accounts: $(patsubst %, query-account-%,$(KEY_NAMES))

.PHONY: query-account-%
query-account-%:
	$(AKASHCTL) query account "$(shell $(AKASHCTL_NONODE) keys show -a "$(@:query-account-%=%)")"

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
