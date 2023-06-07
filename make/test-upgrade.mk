AP_RUN_DIR               := $(AKASH_RUN)/upgrade

AKASH_HOME               := $(AP_RUN_DIR)/.akash
AKASH_INIT               := $(AP_RUN_DIR)/.akash-init

MNEMONIC                 := "wild random elephant refuse clock effort menu barely broccoli team mind magnet pretty fashion fame category turtle rug exclude card view civil purity powder"
TEST_PRIV_KEY            := '{"address":"06DCDACF975BE69C039C62052F9FE0F3906575D1","pub_key":{"type":"tendermint/PubKeyEd25519","value":"d0sS1j4EdrAkBkpFXb50lkibj7+Kwh9UtGPO5O35Pes="},"priv_key":{"type":"tendermint/PrivKeyEd25519","value":"ZVh/Fsra8CKOuGkBT7/dpdAy/dvfLaPeDZZ1suIw2h53SxLWPgR2sCQGSkVdvnSWSJuPv4rCH1S0Y87k7fk96w=="}}'

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
UPGRADE_TO              ?= v0.24.0
UPGRADE_FROM            := $(shell cat $(ROOT_DIR)/meta.json | jq --arg name $(UPGRADE_TO) '.upgrades[$$name].from_version' | tr -d '\n')
GENESIS_BINARY_VERSION  := $(shell cat $(ROOT_DIR)/meta.json | jq --arg name $(UPGRADE_TO) '.upgrades[$$name].from_binary' | tr -d '\n')
UPGRADE_BINARY_VERSION  ?= local

GENESIS_CONFIG_TEMPLATE ?= $(CURDIR)/config-$(UPGRADE_TO).tmpl.json
GENESIS_ORIG            ?= https://github.com/akash-network/testnetify/releases/download/$(UPGRADE_FROM)/genesis.json.tar.lz4
GENESIS_DEST            := $(AKASH_HOME)/config/genesis.json
PRIV_VALIDATOR_KEY      := $(AKASH_HOME)/config/priv_validator_key.json
KEYRING_DIR             := $(AKASH_HOME)/keyring-test
KEY_FILE                := $(KEYRING_DIR)/$(KEY_NAME).info
COSMOVISOR_DIR          := $(AKASH_HOME)/cosmovisor
GENESIS_BINARY_DIR      := $(COSMOVISOR_DIR)/genesis/bin
UPGRADE_BINARY_DIR      := $(COSMOVISOR_DIR)/upgrades/$(UPGRADE_TO)/bin
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

$(GENESIS_DEST): $(GENESIS_BINARY) $(PRIV_VALIDATOR_KEY)
	wget -qO - "$(GENESIS_ORIG)" | lz4 - -d | tar xf - -C $(AKASH_HOME)/config

.PHONY: genesis
genesis: $(GENESIS_DEST)

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
	echo $(TEST_PRIV_KEY) > $(PRIV_VALIDATOR_KEY)

.INTERMEDIATE: keys-init
keys-init: $(patsubst %,$(KEYRING_DIR)/%.info,$(KEY_NAMES))

$(KEYRING_DIR)/%.info:
	echo $(MNEMONIC) | $(GENESIS_BINARY) --home=$(AKASH_HOME) --keyring-backend=test keys add $(@:$(KEYRING_DIR)/%.info=%) --recover

.INTERMEDIATE: node-init-finalize
node-init-finalize:

.PHONY: genesis
genesis: $(GENESIS_DEST)

.PHONY: test
test: init upgrade-binary
	$(GO_TEST) ./... -tags e2e.upgrade -timeout 60m -v -args \
		-home=$(AP_RUN_DIR) \
		-cosmovisor=$(COSMOVISOR) \
		-genesis-binary=$(GENESIS_BINARY) \
		-chain-id="localakash" \
		-upgrade-name=$(UPGRADE_TO) \
		-upgrade-version="$(UPGRADE_BINARY_VERSION)" \
		-test-cases=upgrades-$(UPGRADE_TO).json

.PHONY: test-reset
test-reset:
	rm -rf $(AKASH_HOME)/data/*
	rm -rf $(COSMOVISOR_DIR)/current
	rm -rf $(COSMOVISOR_DIR)/upgrades/$(UPGRADE_TO)/upgrade-info.json
	@echo '{"height":"0","round": 0,"step": 0}' > $(AKASH_HOME)/data/priv_validator_state.json

.PHONY: clean
clean:
	rm -rf $(AP_RUN_DIR)
