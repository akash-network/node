AP_RUN_DIR               := $(AKASH_RUN)/upgrade

AKASH_INIT               := $(AP_RUN_DIR)/.akash-init

export AKASH_HOME
export AKASH_KEYRING_BACKEND    = test
export AKASH_GAS_ADJUSTMENT     = 2
export AKASH_CHAIN_ID           = localakash
export AKASH_YES                = true
export AKASH_GAS_PRICES         = 0.025uakt
export AKASH_GAS                = auto
export AKASH_STATESYNC_ENABLE   = false
export AKASH_LOG_COLOR          = true

KEY_OPTS                := --keyring-backend=$(AKASH_KEYRING_BACKEND)
KEY_NAME                ?= validator
UPGRADE_TO              ?= $(shell $(ROOT_DIR)/script/upgrades.sh test-required $(RELEASE_TAG))
# v0.28.0
UPGRADE_FROM            := $(shell cat $(ROOT_DIR)/meta.json | jq -r --arg name $(UPGRADE_TO) '.upgrades[$$name].from_version' | tr -d '\n')
GENESIS_BINARY_VERSION  := $(shell cat $(ROOT_DIR)/meta.json | jq -r --arg name $(UPGRADE_TO) '.upgrades[$$name].from_binary' | tr -d '\n')
UPGRADE_BINARY_VERSION  ?= local

$(AKASH_INIT):
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --gbv=$(GENESIS_BINARY_VERSION) --ufrom=$(UPGRADE_FROM) --uto=$(UPGRADE_TO) --config="$(PWD)/config.json" init
	touch $@

.PHONY: init
init: $(AKASH_INIT)


.PHONY: genesis
genesis: $(GENESIS_DEST)

.PHONY: test
test: $(COSMOVISOR) init
	$(GO_TEST) -run "^\QTestUpgrade\E$$" -tags e2e.upgrade -timeout 60m -v -args \
		-cosmovisor=$(COSMOVISOR) \
		-workdir=$(AP_RUN_DIR)/validators \
		-config=test-config.json \
		-upgrade-name=$(UPGRADE_TO) \
		-upgrade-version="$(UPGRADE_BINARY_VERSION)" \
		-test-cases=upgrade-$(UPGRADE_TO).json

.PHONY: test-reset
test-reset:
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --config="$(PWD)/config.json" clean

.PHONY: clean
clean:
	rm -rf $(AP_RUN_DIR)
