include ../common.mk
include ../common-commands.mk
include ../common-kind.mk

KUSTOMIZE_ROOT           ?= $(AKASH_ROOT)/_docs/kustomize
KUSTOMIZE_DIR            ?= $(AKASH_ROOT)/_run/single/kustomize
KUSTOMIZE_PROVIDER_DIR   ?= $(KUSTOMIZE_DIR)/akash-provider
KUSTOMIZE_PROVIDER_CACHE ?= $(KUSTOMIZE_PROVIDER_DIR)/cache
KUSTOMIZE_AKASHD_DIR     ?= $(KUSTOMIZE_DIR)/akash-node
KUSTOMIZE_AKASHD_CACHE   ?= $(KUSTOMIZE_AKASHD_DIR)/cache
CLIENT_EXPORT_PASSWORD   ?= 12345678

PROVIDER_HOSTNAME  = akash-provider.localhost
export AKASH_NODE  = tcp://akash.localhost:$(KIND_PORT_BINDINGS)
GATEWAY_ENDPOINT  ?= https://akash-provider.localhost

.PHONY: kind-namespace-setup
kind-namespace-setup:
	kubectl apply -f "$(KUSTOMIZE_ROOT)/networking"

.PHONY: kustomize-init
kustomize-init: kustomize-init-akash-node kustomize-init-provider kind-namespace-setup

.PHONY: kustomize-init-akash-node
kustomize-init-akash-node:
	mkdir -p "$(KUSTOMIZE_AKASHD_CACHE)"
	cp -r "$(AKASH_HOME)/"* "$(KUSTOMIZE_AKASHD_CACHE)/"

.PHONY: kustomize-init-provider
kustomize-init-provider:
	mkdir -p "$(KUSTOMIZE_PROVIDER_CACHE)"
	cp -r "$(AKASH_HOME)/config" "$(KUSTOMIZE_PROVIDER_CACHE)/"
	echo "$(CLIENT_EXPORT_PASSWORD)" > "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt"
	cat "$(AKASH_HOME)/$(PROVIDER_ADDRESS).pem" > "$(KUSTOMIZE_PROVIDER_CACHE)/provider-cert.pem"
	( \
		cat "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt" ; \
		cat "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt"   \
	) | $(AKASH) keys export provider 1> "$(KUSTOMIZE_PROVIDER_CACHE)/key.txt"

.PHONY: kustomize-install-node
kustomize-install-node:
	kubectl kustomize kustomize/akash-node | kubectl apply -f-

.PHONY: kustomize-install-provider
kustomize-install-provider:
	kubectl kustomize kustomize/akash-provider | kubectl apply -f-

.PHONY: provider-lease-ping
provider-lease-ping:
	curl -sIH "Host: hello.localhost" localhost:$(KIND_HTTP_PORT)

PHONY: provider-lease-status
provider-lease-status:
	$(AKASH) provider lease-status \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--from      "$(KEY_NAME)" \
		--provider  "$(PROVIDER_ADDRESS)"

PHONY: clean-$(AKASH_RUN_NAME)
clean-$(AKASH_RUN_NAME):
	rm -rf $(CURDIR)/kustomize/akash-node/cache
	rm -f  $(CURDIR)/kustomize/akash-node/docker-image.yaml
	rm -rf $(CURDIR)/kustomize/akash-provider/cache
	rm -f  $(CURDIR)/kustomize/akash-provider/docker-image.yaml
