include ../common.mk
include ../common-commands.mk
include kind.mk

GATEWAY_HOST     ?= localhost:8080
GATEWAY_ENDPOINT ?= http://$(GATEWAY_HOST)

.PHONY: provider-run
provider-run:
	AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS="false" \
		$(AKASHCTL) $(KEY_OPTS) provider run \
			--from "$(PROVIDER_KEY_NAME)" \
			--cluster-k8s \
			--gateway-listen-address "$(GATEWAY_HOST)"

.PHONY: provider-status
provider-status:
	curl -s "$(GATEWAY_ENDPOINT)/status" | jq

.PHONY: provider-send-manifest
provider-send-manifest:
	$(AKASHCTL) provider send-manifest "$(SDL_PATH)" \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

.PHONY: provider-lease-status
provider-lease-status:
	curl -s \
		"$(GATEWAY_ENDPOINT)/lease/$(KEY_ADDRESS)/$(DSEQ)/$(GSEQ)/$(OSEQ)/$(PROVIDER_ADDRESS)/status" | \
		jq

.PHONY: provider-lease-ping
provider-lease-ping:
	curl -sIH "Host: hello.localhost" localhost:8081
