include ../common.mk

NODE_CHART = akash-node
NODE_FILES = $(sort $(wildcard $(NODE_ROOT)/*.yaml))
NODES      = $(patsubst $(NODE_ROOT)/%.yaml,%,$(NODE_FILES))

PROVIDER_CHART = akash-provider
PROVIDERS      = us-west-1 us-west-2 us-east-1 # us-east-2 ap-southeast-1 ap-southeast-2

helm-install-nodes: $(patsubst %,helm-install-node-%,$(NODES))
helm-upgrade-nodes: $(patsubst %,helm-upgrade-node-%,$(NODES))
helm-check-nodes:   $(patsubst %,helm-check-node-%,$(NODES))
helm-delete-nodes:  $(patsubst %,helm-delete-node-%,$(NODES))
helm-reset-nodes:   helm-delete-nodes helm-install-nodes

helm-install-node-%:
	helm install "$(NODE_CHART)"                         \
		-n "$(@:helm-install-node-%=%)"                    \
		--set "ingress.domain=$(shell minikube ip).nip.io" \
		-f "$(NODE_ROOT)/$(@:helm-install-node-%=%).yaml"

helm-upgrade-node-%:
	helm upgrade "$(@:helm-upgrade-node-%=%)" "$(NODE_CHART)" \
		--recreate-pods                                         \
		--set "ingress.domain=$(shell minikube ip).nip.io"      \
		-f "$(NODE_ROOT)/$(@:helm-upgrade-node-%=%).yaml"

helm-check-node-%:
	curl -f "$(@:helm-check-node-%=%).$(shell minikube ip).nip.io/status"
	@echo

helm-delete-node-%:
	helm delete "$(@:helm-delete-node-%=%)" --purge

helm-install-providers: $(patsubst %,helm-install-provider-%,$(PROVIDERS))
helm-upgrade-providers: $(patsubst %,helm-upgrade-provider-%,$(PROVIDERS))
helm-check-providers:   $(patsubst %,helm-check-provider-%,$(PROVIDERS))
helm-delete-providers:  $(patsubst %,helm-delete-provider-%,$(PROVIDERS))
helm-reset-providers:   helm-delete-providers helm-install-providers

helm-install-provider-%:
	helm install "$(PROVIDER_CHART)"                     \
		-n "$(@:helm-install-provider-%=%)"                \
		--set "ingress.domain=$(shell minikube ip).nip.io" \
		--set "deployment.ingress.domain=$(shell minikube ip).nip.io" \
		--set "provider.region=$(shell echo "$@" | sed -e 's/.*-\(.*-.*\)-[0-9][0-9]*/\1/')"

helm-upgrade-provider-%:
	helm upgrade "$(@:helm-upgrade-provider-%=%)" "$(PROVIDER_CHART)" \
		--recreate-pods                                                 \
		--set "ingress.domain=$(shell minikube ip).nip.io"              \
		--set "deployment.ingress.domain=$(shell minikube ip).nip.io" \
		--set "provider.region=$(shell echo "$@" | sed -e 's/.*-\(.*-.*\)-[0-9][0-9]*/\1/')"

helm-delete-provider-%:
	helm delete "$(@:helm-delete-provider-%=%)" --purge

helm-check-provider-%:
	curl -f "$(@:helm-check-provider-%=%).$(shell minikube ip).nip.io/status"
	@echo
