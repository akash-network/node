PHOTON  = ../photon
PHOTOND = ../photond

DATA_ROOT   = data
MASTER_KEY  = $(DATA_ROOT)/master.key
OTHER_KEY   = $(DATA_ROOT)/other.key
CLIENT_ROOT = $(DATA_ROOT)/client
NODE_ROOT   = $(DATA_ROOT)/node

HELM_CHART = photon-node

HELM_NODE_COUNT ?= 4
HELM_NODE_FILES = $(sort $(wildcard $(NODE_ROOT)/*.yaml))
HELM_NODES      = $(patsubst $(NODE_ROOT)/%.yaml,%,$(HELM_NODE_FILES))

image-minikube:
	( cd .. && make image-minikube )

keys: $(MASTER_KEY) $(OTHER_KEY)

$(MASTER_KEY):
	mkdir -p "$(DATA_ROOT)"
	$(PHOTON) key create master -d "$(CLIENT_ROOT)" > "$(MASTER_KEY)"

$(OTHER_KEY):
	mkdir -p "$(DATA_ROOT)"
	$(PHOTON) key create other -d "$(CLIENT_ROOT)" > "$(OTHER_KEY)"

genesis: $(MASTER_KEY)
	$(PHOTOND) init $$(cat "$(MASTER_KEY)") -d "$(NODE_ROOT)"

genesis-helm: $(MASTER_KEY)
	$(PHOTOND) init $$(cat "$(MASTER_KEY)") -d "$(NODE_ROOT)" -t helm -c "$(HELM_NODE_COUNT)"

helm-init: keys genesis-helm

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

clean:
	rm -rf data
