include ../common.mk
include ../common-commands.mk
include ../common-kind.mk

GATEWAY_HOSTNAME ?= localhost
GATEWAY_HOST     ?= $(GATEWAY_HOSTNAME):8443
GATEWAY_ENDPOINT ?= https://$(GATEWAY_HOST)

.PHONY: provider-run
provider-run:
	$(AKASHCTL) $(KEY_OPTS) provider run \
		--chain-id "$(CHAIN_NAME)" \
		--from "$(PROVIDER_KEY_NAME)" \
		--cluster-k8s \
		--gateway-listen-address "$(GATEWAY_HOST)" \
		--deployment-ingress-static-hosts true \
		--deployment-ingress-domain "$(GATEWAY_HOSTNAME)" \
		--cluster-node-port-quantity 100 \
		--cluster-public-hostname "$(GATEWAY_HOSTNAME)" \
		--bid-price-strategy "randomRange"

.PHONY: provider-status
provider-status:
	$(AKASHCTL) provider status --provider "$(PROVIDER_ADDRESS)"

.PHONY: provider-lease-status
provider-lease-status:
	$(AKASHCTL) provider lease-status \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-service-status
provider-service-status:
	$(AKASHCTL) provider service-status   \
		--owner     "$(KEY_ADDRESS)"      \
		--dseq      "$(DSEQ)"             \
		--gseq      "$(GSEQ)"             \
		--oseq      "$(OSEQ)"             \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-service-logs
provider-service-logs:
	$(AKASHCTL) provider service-logs     \
		--owner     "$(KEY_ADDRESS)"      \
		--dseq      "$(DSEQ)"             \
		--gseq      "$(GSEQ)"             \
		--oseq      "$(OSEQ)"             \
		--provider  "$(PROVIDER_ADDRESS)" \
		--follow    false                 \
		--service   "web"                 \
		--tail      "100"

.PHONY: provider-lease-ping
provider-lease-ping:
	curl -sIH "Host: hello.localhost" localhost:$(KIND_HTTP_PORT)
