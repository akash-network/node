include ../common-base.mk

# https://stackoverflow.com/a/7531247
# https://www.gnu.org/software/make/manual/make.html#Flavors
null  := 
space := $(null) #
comma := ,

DATA_ROOT    := ./cache
NODE_HOME    := $(DATA_ROOT)
CLIENT_HOME  := $(DATA_ROOT)

export AKASH_KEYRING_BACKEND = test
export AKASH_GAS_ADJUSTMENT  = 2
export AKASH_CHAIN_ID        = local
export AKASH_YES             = true

KEY_OPTS     := --keyring-backend=$(AKASH_KEYRING_BACKEND)
GENESIS_PATH := $(NODE_HOME)/config/genesis.json

CHAIN_MIN_DEPOSIT     := 10000000000000
CHAIN_ACCOUNT_DEPOSIT := $(shell echo $$(($(CHAIN_MIN_DEPOSIT) * 10)))
CHAIN_TOKEN_DENOM     := uakt

AKASHCTL_NONODE := $(AKASH_BIN) --home "$(CLIENT_HOME)"
AKASHCTL := $(AKASHCTL_NONODE)
AKASHD   := $(AKASH_BIN)   --home "$(NODE_HOME)"

KEY_NAMES := main provider validator other

MULTISIG_KEY     := msig
MULTISIG_SIGNERS := main other

GENESIS_ACCOUNTS := $(KEY_NAMES) $(MULTISIG_KEY)

CLIENT_CERTS := main validator other
SERVER_CERTS := provider


.PHONY: init
init: bins client-init node-init

.PHONY: client-init
client-init: init-dirs client-init-keys

.PHONY: init-dirs
init-dirs: 
	mkdir -p "$(CLIENT_HOME)" "$(NODE_HOME)"

.PHONY: client-init-keys
client-init-keys: $(patsubst %,client-init-key-%,$(KEY_NAMES)) client-init-multisig-key

.PHONY: client-init-key-%
client-init-key-%:
	$(AKASHCTL_NONODE) keys add "$(@:client-init-key-%=%)"

.PHONY: client-init-multisig-key
client-init-multisig-key:
	$(AKASHCTL_NONODE) keys add \
		"$(MULTISIG_KEY)" \
		--multisig "$(subst $(space),$(comma),$(strip $(MULTISIG_SIGNERS)))" \
		--multisig-threshold 2

.PHONY: node-init
node-init: node-init-genesis node-init-genesis-accounts node-init-genesis-certs node-init-gentx node-init-finalize

.PHONY: node-init-genesis
node-init-genesis: init-dirs
	$(AKASHD) init node0 
	cp "$(GENESIS_PATH)" "$(GENESIS_PATH).orig"
	cat "$(GENESIS_PATH).orig" | \
        jq -M '.app_state.gov.voting_params.voting_period = "15s"' | \
		jq -rM '(..|objects|select(has("denom"))).denom           |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("bond_denom"))).bond_denom |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("mint_denom"))).mint_denom |= "$(CHAIN_TOKEN_DENOM)"' > \
		"$(GENESIS_PATH)"

.PHONY: node-init-genesis-certs
node-init-genesis-certs: $(patsubst %,node-init-genesis-client-cert-%,$(CLIENT_CERTS)) $(patsubst %,node-init-genesis-server-cert-%,$(SERVER_CERTS))

.PHONY: node-init-genesis-client-cert-%
node-init-genesis-client-cert-%:
	$(AKASHD) tx cert create client --to-genesis=true --from=$(@:node-init-genesis-client-cert-%=%)

.PHONY: node-init-genesis-server-cert-%
node-init-genesis-server-cert-%:
	$(AKASHD) tx cert create server localhost akash-provider.localhost --to-genesis=true --from=$(@:node-init-genesis-server-cert-%=%)

.PHONY: node-init-genesis-accounts
node-init-genesis-accounts: $(patsubst %,node-init-genesis-account-%,$(GENESIS_ACCOUNTS))
	$(AKASHD) validate-genesis

.PHONY: node-init-genesis-account-%
node-init-genesis-account-%:
	$(AKASHD) add-genesis-account --keyring-backend test \
		"$(shell $(AKASHCTL_NONODE) $(KEY_OPTS) keys show "$(@:node-init-genesis-account-%=%)" -a)" \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.PHONY: node-init-gentx
node-init-gentx:
	$(AKASHD) gentx validator \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

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
