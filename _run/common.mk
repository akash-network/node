include ../common-base.mk

# https://stackoverflow.com/a/7531247
# https://www.gnu.org/software/make/manual/make.html#Flavors
null  := 
space := $(null) #
comma := ,

ifndef AKASH_HOME
$(error AKASH_HOME is not set)
endif

export AKASH_KEYRING_BACKEND = test
export AKASH_GAS_ADJUSTMENT  = 2
export AKASH_CHAIN_ID        = local
export AKASH_YES             = true

AKASH        := $(AKASH) --home $(AKASH_HOME)

KEY_OPTS     := --keyring-backend=$(AKASH_KEYRING_BACKEND)
GENESIS_PATH := $(AKASH_HOME)/config/genesis.json

CHAIN_MIN_DEPOSIT     := 10000000000000
CHAIN_ACCOUNT_DEPOSIT := $(shell echo $$(($(CHAIN_MIN_DEPOSIT) * 10)))
CHAIN_TOKEN_DENOM     := uakt

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
	mkdir -p "$(AKASH_HOME)"

.PHONY: client-init-keys
client-init-keys: $(patsubst %,client-init-key-%,$(KEY_NAMES)) client-init-multisig-key

.PHONY: client-init-key-%
client-init-key-%:
	$(AKASH) keys add "$(@:client-init-key-%=%)"

.PHONY: client-init-multisig-key
client-init-multisig-key:
	$(AKASH) keys add \
		"$(MULTISIG_KEY)" \
		--multisig "$(subst $(space),$(comma),$(strip $(MULTISIG_SIGNERS)))" \
		--multisig-threshold 2

.PHONY: node-init
node-init: node-init-genesis node-init-genesis-accounts node-init-genesis-certs node-init-gentx node-init-finalize

.PHONY: node-init-genesis
node-init-genesis: init-dirs
	$(AKASH) init node0
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
	$(AKASH) tx cert generate client --from=$*
	$(AKASH) tx cert publish client --to-genesis=true --from=$*

.PHONY: node-init-genesis-server-cert-%
node-init-genesis-server-cert-%:
	$(AKASH) tx cert generate server localhost akash-provider.localhost --from=$*
	$(AKASH) tx cert publish server --to-genesis=true --from=$*

.PHONY: node-init-genesis-accounts
node-init-genesis-accounts: $(patsubst %,node-init-genesis-account-%,$(GENESIS_ACCOUNTS))
	$(AKASH) validate-genesis

.PHONY: node-init-genesis-account-%
node-init-genesis-account-%:
	$(AKASH) add-genesis-account \
		"$(shell $(AKASH) $(KEY_OPTS) keys show "$(@:node-init-genesis-account-%=%)" -a)" \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.PHONY: node-init-gentx
node-init-gentx:
	$(AKASH) gentx validator \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.PHONY: node-init-finalize
node-init-finalize:
	$(AKASH) collect-gentxs
	$(AKASH) validate-genesis

.PHONY: node-run
node-run:
	$(AKASH) start

.PHONY: node-status
node-status:
	$(AKASH) status

.PHONY: rest-server-run
rest-server-run:
	$(AKASH) rest-server

.PHONY: clean
clean: clean-$(AKASH_RUN_NAME)
	rm -rf "$(AKASH_HOME)"

.PHONY: rosetta-run
rosetta-run:
	$(AKASH) rosetta --addr localhost:8080 --grpc localhost:9090 --network=$(AKASH_CHAIN_ID) --blockchain=akash
