include ../common.mk
include ../common-commands.mk

provider-run:
	$(AKASHCTL) $(KEY_OPTS) provider run \
		--from "$(PROVIDER_KEY_NAME)" \
		--cluster-k8s

provider-status:
	curl -s http://localhost:8080/status | jq

send-to-main:
	$(akashctl) tx send $(OTHR_ADDR) $(MAIN_ADDR) 117akash $(KEY_OPTS) $(CHAIN_OPTS) --memo "send monies to main" -y --fees 10akash --trace
