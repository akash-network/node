include ../common.mk
include ../common-commands.mk

PROVIDER_HOSTNAME ?= https://localhost:8443

provider-run:
	$(AKASH) provider run \
		--from "$(PROVIDER_KEY_NAME)" \
		--cluster-public-hostname "localhost" \
		--bid-price-strategy "randomRange"

.PHONY: provider-lease-status
provider-lease-status:
	$(AKASH) provider lease-status \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-service-status
provider-service-status:
	$(AKASH) provider service-status \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--service   "web"            \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: clean-lite
clean-lite:
