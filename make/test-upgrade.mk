AP_RUN_DIR               := $(AKASH_RUN)/upgrade

AKASH_HOME               := $(AP_RUN_DIR)/.akash
AKASH_INIT               := $(AP_RUN_DIR)/.akash-init

CHAIN_TOKEN_DENOM        := uakt
CHAIN_VALIDATOR_AMOUNT   := 20000000000000000
CHAIN_VALIDATOR_DELEGATE := 15000000000000000

TESTNETIFY_CONFIG        := $(AP_RUN_DIR)/config.json

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
UPGRADE_NAME            ?= v0.24.0
GENESIS_BINARY_VERSION  ?= v0.22.6
UPGRADE_BINARY_VERSION  ?= local

GENESIS_CONFIG_TEMPLATE ?= $(CURDIR)/config-$(UPGRADE_NAME).tmpl.json


GENESIS_ORIG            ?= $(ROOT_DIR)/genesis.json
GENESIS_DEST            := $(AKASH_HOME)/config/genesis.json
PRIV_VALIDATOR_KEY      := $(AKASH_HOME)/config/priv_validator_key.json
KEYRING_DIR             := $(AKASH_HOME)/keyring-test
KEY_FILE                := $(KEYRING_DIR)/$(KEY_NAME).info
COSMOVISOR_DIR          := $(AKASH_HOME)/cosmovisor
GENESIS_BINARY_DIR      := $(COSMOVISOR_DIR)/genesis/bin
UPGRADE_BINARY_DIR      := $(COSMOVISOR_DIR)/upgrades/$(UPGRADE_NAME)/bin
GENESIS_BINARY          := $(GENESIS_BINARY_DIR)/akash
UPGRADE_BINARY          := $(UPGRADE_BINARY_DIR)/akash

KEY_NAMES        := validator
GENESIS_ACCOUNTS := $(KEY_NAMES)

$(AKASH_HOME):
	mkdir -p $(COSMOVISOR_DIR)/genesis/bin

$(AKASH_INIT): $(AKASH_HOME) $(COSMOVISOR) binaries node-init keys-init $(GENESIS_DEST) node-init-finalize
	touch $@

.INTERMEDIATE: init
init: $(AKASH_INIT)

$(GENESIS_DEST): SHELL=$(BASH_PATH)
$(GENESIS_DEST): $(GENESIS_BINARY) $(TESTNETIFY_CONFIG) $(PRIV_VALIDATOR_KEY)
	$(GENESIS_BINARY) debug testnetify $(GENESIS_ORIG) $(GENESIS_DEST) -c $(TESTNETIFY_CONFIG)

$(TESTNETIFY_CONFIG): $(GENESIS_BINARY) $(GOMPLATE) $(GENESIS_CONFIG_TEMPLATE)
	$(ROOT_DIR)/script/testnetify-render-config.sh \
		$(GENESIS_BINARY) \
		$(KEY_NAME) \
		config-$(UPGRADE_NAME).tmpl.json \
		$(TESTNETIFY_CONFIG)

.INTERMEDIATE: genesis-binary
.INTERMEDIATE: upgrade-binary
$(GENESIS_BINARY):
	$(ROOT_DIR)/install.sh -b "$(GENESIS_BINARY_DIR)" $(GENESIS_BINARY_VERSION)
	chmod +x $(GENESIS_BINARY)

ifeq ($(UPGRADE_BINARY_VERSION), local)
$(UPGRADE_BINARY): AKASH=$(UPGRADE_BINARY)
$(UPGRADE_BINARY):
	mkdir -p $(UPGRADE_BINARY_DIR)
	make make -sC $(ROOT_DIR) akash

.PHONY: clean-upgrade-binary
clean-upgrade-binary:
	rm -f $(UPGRADE_BINARY)
upgrade-binary: clean-upgrade-binary $(UPGRADE_BINARY)
endif

genesis-binary: $(GENESIS_BINARY)
upgrade-binary:

.INTERMEDIATE:
binaries: genesis-binary upgrade-binary

.INTERMEDIATE: node-init
node-init: $(PRIV_VALIDATOR_KEY)

$(PRIV_VALIDATOR_KEY): $(GENESIS_BINARY)
	$(GENESIS_BINARY) init --home=$(AKASH_HOME) upgrade-validator >/dev/null 2>&1
	rm $(GENESIS_DEST)

.INTERMEDIATE: keys-init
keys-init: $(patsubst %,$(KEYRING_DIR)/%.info,$(KEY_NAMES))

$(KEYRING_DIR)/%.info:
	$(GENESIS_BINARY) --home=$(AKASH_HOME) --keyring-backend=test keys add $(@:$(KEYRING_DIR)/%.info=%)

.INTERMEDIATE: node-init-finalize
node-init-finalize:
	#$(GENESIS_BINARY) validate-genesis

.PHONY: keys-list
keys-list:
	$(GENESIS_BINARY) keys list

.PHONY: genesis
genesis: $(GENESIS_DEST)

.PHONY: config
config: $(TESTNETIFY_CONFIG)

.PHONY: test
test: init #upgrade-binary
	$(GO) test ./... -timeout 60m -v -args \
		-home=$(AP_RUN_DIR) \
		-cosmovisor=$(COSMOVISOR) \
		-genesis-binary=$(GENESIS_BINARY) \
		-chain-id="localakash" \
		-upgrade-name=$(UPGRADE_NAME) \
		-upgrade-version="$(UPGRADE_BINARY_VERSION)" \
		-test-cases=upgrades-$(UPGRADE_NAME).json

.PHONY: test-reset
test-reset:
	rm -rf $(AKASH_HOME)/data/*
	rm -rf $(COSMOVISOR_DIR)/current
	rm -rf $(COSMOVISOR_DIR)/upgrades/$(UPGRADE_NAME)/upgrade-info.json
	@echo '{"height":"0","round": 0,"step": 0}' > $(AKASH_HOME)/data/priv_validator_state.json

.PHONY: clean
clean:
	rm -rf $(AP_RUN_DIR)
