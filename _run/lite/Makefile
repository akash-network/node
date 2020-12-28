include ../common.mk
include ../common-commands.mk

PROVIDER_HOSTNAME ?= https://localhost:8443

provider-run:
	$(AKASHCTL) $(KEY_OPTS) provider run $(CHAIN_OPTS) \
		--from "$(PROVIDER_KEY_NAME)" \
		--cluster-public-hostname "localhost" \
		--bid-price-strategy "randomRange"

.PHONY: provider-status
provider-status:
	$(AKASHCTL) provider status $(PROVIDER_ADDRESS)

.PHONY: provider-lease-status
provider-lease-status:
	$(AKASHCTL) "$(KEY_OPTS)" provider lease-status \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-service-status
provider-service-status:
	$(AKASHCTL) "$(KEY_OPTS)" provider service-status \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--service   "web"            \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-service-logs
provider-service-logs:
	$(AKASHCTL) "$(KEY_OPTS)" provider service-logs \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--service   "web"            \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"
