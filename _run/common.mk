include ../common-base.mk

DATA_ROOT    := $(PWD)/cache
NODE_HOME    := $(DATA_ROOT)/node
CLIENT_HOME  := $(DATA_ROOT)/client

CHAIN_NAME   := local
CHAIN_OPTS   := --chain-id $(CHAIN_NAME)
GENESIS_PATH := $(NODE_HOME)/config/genesis.json

CHAIN_MIN_DEPOSIT     := 10000000
CHAIN_ACCOUNT_DEPOSIT := $(shell echo $$(($(CHAIN_MIN_DEPOSIT) * 10)))
CHAIN_TOKEN_DENOM     := akash

AKASHCTL := $(AKASHCTL_BIN) --home "$(CLIENT_HOME)"
AKASHD   := $(AKASHD_BIN)   --home "$(NODE_HOME)"
KEY_OPTS := --keyring-backend=test

KEY_NAMES := main provider validator other

.PHONY: init
init: bins client-init node-init

.PHONY: client-init
client-init: init-dirs client-init-config client-init-keys

.PHONY: init-dirs
init-dirs: 
	mkdir -p "$(CLIENT_HOME)" "$(NODE_HOME)"

.PHONY: client-init-config
client-init-config:
	$(AKASHCTL) config chain-id        "$(CHAIN_NAME)"
	$(AKASHCTL) config output          json
	$(AKASHCTL) config indent          true
	$(AKASHCTL) config trust-node      true
	$(AKASHCTL) config keyring-backend test
	$(AKASHCTL) config broadcast-mode  block

.PHONY: client-init-keys
client-init-keys: $(patsubst %,client-init-key-%,$(KEY_NAMES))

.PHONY: client-init-key-%
client-init-key-%:
	$(AKASHCTL) keys add "$(@:client-init-key-%=%)"

.PHONY: node-init
node-init: node-init-genesis node-init-genesis-accounts node-init-gentx node-init-finalize

.PHONY: node-init-genesis
node-init-genesis: init-dirs
	$(AKASHD) init node0 $(CHAIN_OPTS)
	cp "$(GENESIS_PATH)" "$(GENESIS_PATH).orig"
	cat "$(GENESIS_PATH).orig" | \
		jq -rM '(..|objects|select(has("denom"))).denom           |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("bond_denom"))).bond_denom |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("mint_denom"))).mint_denom |= "$(CHAIN_TOKEN_DENOM)"' > \
		"$(GENESIS_PATH)"

.PHONY: node-init-genesis-accounts
node-init-genesis-accounts: $(patsubst %,node-init-genesis-account-%,$(KEY_NAMES))
	$(AKASHD) validate-genesis

.PHONY: node-init-genesis-account-%
node-init-genesis-account-%:
	$(AKASHD) add-genesis-account \
		"$(shell $(AKASHCTL) keys show "$(@:node-init-genesis-account-%=%)" -a)" \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.PHONY: node-init-gentx
node-init-gentx:
	$(AKASHD) $(KEY_OPTS) gentx      \
		--name validator               \
		--home-client "$(CLIENT_HOME)" \
		--amount "$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.PHONY: node-init-finalize
node-init-finalize:
	$(AKASHD) collect-gentxs
	$(AKASHD) validate-genesis

.PHONY: node-run
node-run:
	$(AKASHD) start

.PHONY: node-status
node-status:
	$(AKASHCTL) status

.PHONY: rest-server-run
rest-server-run:
	$(AKASHCTL) rest-server

.PHONY: clean
clean:
	rm -rf "$(DATA_ROOT)"
