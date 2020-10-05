include ../common.mk
include ../common-commands.mk

PROVIDER_HOSTNAME ?= http://localhost:8080

provider-run:
	$(AKASHCTL) $(KEY_OPTS) provider run $(CHAIN_OPTS) \
		--from "$(PROVIDER_KEY_NAME)"

provider-status:
	curl -s "$(PROVIDER_HOSTNAME)/status" | jq -r
