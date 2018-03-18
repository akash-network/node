include ../common.mk

HELM_CHART = akash-node

HELM_NODE_FILES = $(sort $(wildcard $(NODE_ROOT)/*.yaml))
HELM_NODES      = $(patsubst $(NODE_ROOT)/%.yaml,%,$(HELM_NODE_FILES))

helm-install: $(patsubst %,helm-install-%,$(HELM_NODES))
helm-upgrade: $(patsubst %,helm-upgrade-%,$(HELM_NODES))
helm-delete:  $(patsubst %,helm-delete-%,$(HELM_NODES))
helm-reset:   helm-delete helm-install

helm-install-node-%:
	helm install "$(HELM_CHART)" -n "$(@:helm-install-%=%)" -f "$(NODE_ROOT)/$(@:helm-install-%=%).yaml"

helm-upgrade-node-%:
	helm upgrade "$(@:helm-upgrade-%=%)" "$(HELM_CHART)" -f "$(NODE_ROOT)/$(@:helm-upgrade-%=%).yaml"

helm-delete-node-%:
	helm delete "$(@:helm-delete-%=%)" --purge

helm-install-provider:
	helm install akash-provider -n "akash-provider"
helm-delete-provider:
	helm delete akash-provider --purge
helm-upgrade-provider:
	helm upgrade akash-provider akash-provider
