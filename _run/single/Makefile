include ../common.mk
include ../common-commands.mk
include ../common-kind.mk

KUSTOMIZE_ROOT           ?= ../../_docs/kustomize
KUSTOMIZE_DIR            ?= ./kustomize
KUSTOMIZE_PROVIDER_DIR   ?= $(KUSTOMIZE_DIR)/akash-provider
KUSTOMIZE_PROVIDER_CACHE ?= $(KUSTOMIZE_PROVIDER_DIR)/cache
KUSTOMIZE_AKASHD_DIR     ?= $(KUSTOMIZE_DIR)/akash-node
KUSTOMIZE_AKASHD_CACHE   ?= $(KUSTOMIZE_AKASHD_DIR)/cache
CLIENT_EXPORT_PASSWORD   ?= 12345678

PROVIDER_HOSTNAME = akash-provider.localhost
AKASHCTL_NODE     = "tcp://akash.localhost:$(KIND_PORT_BINDINGS)"
GATEWAY_ENDPOINT ?= http://akash-provider.localhost

AKASHCTL += --node $(AKASHCTL_NODE)

.PHONY: kind-namespace-setup
kind-namespace-setup:
	kubectl apply -f "$(KUSTOMIZE_ROOT)/networking"

.PHONY: kustomize-init
kustomize-init: kustomize-init-akash-node kustomize-init-provider kind-namespace-setup


.PHONY: kustomize-init-akash-node
kustomize-init-akash-node:
	mkdir -p "$(KUSTOMIZE_AKASHD_CACHE)"
	cp -r "$(NODE_HOME)/"* "$(KUSTOMIZE_AKASHD_CACHE)/"

.PHONY: kustomize-init-provider
kustomize-init-provider:
	mkdir -p "$(KUSTOMIZE_PROVIDER_CACHE)"
	cp -r "$(CLIENT_HOME)/config" "$(KUSTOMIZE_PROVIDER_CACHE)/"
	echo "$(CLIENT_EXPORT_PASSWORD)" > "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt"
	( \
		cat "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt" ; \
		cat "$(KUSTOMIZE_PROVIDER_CACHE)/key-pass.txt"   \
	) | $(AKASHCTL_NONODE) $(KEY_OPTS) keys export provider 2> "$(KUSTOMIZE_PROVIDER_CACHE)/key.txt"

.PHONY: kustomize-install-node
kustomize-install-node:
	kubectl kustomize kustomize/akash-node | kubectl apply -f-

.PHONY: kustomize-install-provider
kustomize-install-provider:
	kubectl kustomize kustomize/akash-provider | kubectl apply -f-

.PHONY: send-manifest
send-manifest:
	$(AKASHCTL) provider send-manifest "$(SDL_PATH)" \
		--owner     "$(KEY_ADDRESS)" \
		--dseq      "$(DSEQ)"        \
		--gseq      "$(GSEQ)"        \
		--oseq      "$(OSEQ)"        \
		--provider  "$(PROVIDER_ADDRESS)"

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

.PHONY: provider-lease-ping
provider-lease-ping:
	curl -sIH "Host: hello.localhost" localhost:$(KIND_HTTP_PORT)

.PHONY:
clean-all: clean
	git clean -fdX
