OPTIONS ?=

SKIP_BUILD := false

# check for nostrip option
ifneq (,$(findstring nobuild,$(OPTIONS)))
	SKIP_BUILD := true
endif

include ../common-base.mk

# https://stackoverflow.com/a/7531247
# https://www.gnu.org/software/make/manual/make.html#Flavors
null  := 
space := $(null) #
comma := ,

export AKASH_KEYRING_BACKEND    = test
export AKASH_GAS_ADJUSTMENT     = 2
export AKASH_CHAIN_ID           = local
export AKASH_YES                = true
export AKASH_GAS_PRICES         = 0.025uakt
export AKASH_GAS                = auto
export AKASH_NODE               = http://localhost:26657

AKASH_INIT                     := $(AKASH_RUN_DIR)/.akash-init

KEY_OPTS     := --keyring-backend=$(AKASH_KEYRING_BACKEND)
GENESIS_PATH := $(AKASH_HOME)/config/genesis.json

CHAIN_MIN_DEPOSIT        := 10000000000000
CHAIN_ACCOUNT_DEPOSIT    := $(shell echo $$(($(CHAIN_MIN_DEPOSIT) * 10)))
CHAIN_VALIDATOR_DELEGATE := $(shell echo $$(($(CHAIN_MIN_DEPOSIT) / 2)))
CHAIN_TOKEN_DENOM        := uakt

KEY_NAMES := main provider validator other

MULTISIG_KEY     := msig
MULTISIG_SIGNERS := main other

GENESIS_ACCOUNTS := $(KEY_NAMES) $(MULTISIG_KEY)

CLIENT_CERTS := main validator other
SERVER_CERTS := provider

.PHONY: init
init: bins akash-init

$(AP_RUN_DIR):
	mkdir -p $@

$(AKASH_HOME):
	mkdir -p $@

$(AKASH_INIT): $(AKASH_HOME) client-init node-init
	touch $@

.INTERMEDIATE: akash-init
akash-init: $(AKASH_INIT)

.INTERMEDIATE: client-init
client-init: client-init-keys

.INTERMEDIATE: client-init-keys
client-init-keys: $(patsubst %,client-init-key-%,$(KEY_NAMES)) client-init-key-multisig

.INTERMEDIATE: $(patsubst %,client-init-key-%,$(KEY_NAMES))
client-init-key-%:
	$(AKASH) keys add "$(@:client-init-key-%=%)"

.INTERMEDIATE: client-init-key-multisig
client-init-key-multisig:
	$(AKASH) keys add \
		"$(MULTISIG_KEY)" \
		--multisig "$(subst $(space),$(comma),$(strip $(MULTISIG_SIGNERS)))" \
		--multisig-threshold 2

.NOTPARALLEL: node-init
.INTERMEDIATE: node-init
node-init: node-init-genesis node-init-genesis-accounts node-init-genesis-certs node-init-gentx node-init-finalize

.INTERMEDIATE: node-init-genesis
node-init-genesis:
	$(AKASH) genesis init node0
	cp "$(GENESIS_PATH)" "$(GENESIS_PATH).orig"
	cat "$(GENESIS_PATH).orig" | \
		jq -M '.app_state.gov.voting_params.voting_period = "60s"' | \
		jq -M '.app_state.gov.params.voting_period = "60s"' | \
		jq -M '.app_state.gov.params.expedited_voting_period = "30s"' | \
		jq -M '.app_state.gov.params.max_deposit_period = "60s"' | \
		jq -rM '(..|objects|select(has("denom"))).denom           |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("bond_denom"))).bond_denom |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("mint_denom"))).mint_denom |= "$(CHAIN_TOKEN_DENOM)"' > \
		"$(GENESIS_PATH)"

.INTERMEDIATE: node-init-genesis-certs
node-init-genesis-certs: $(patsubst %,node-init-genesis-client-cert-%,$(CLIENT_CERTS)) $(patsubst %,node-init-genesis-server-cert-%,$(SERVER_CERTS))

.INTERMEDIATE: $(patsubst %,node-init-genesis-client-cert-%,$(CLIENT_CERTS))
node-init-genesis-client-cert-%:
	$(AKASH) tx cert generate client --from=$*
	$(AKASH) tx cert publish client --to-genesis=true --from=$*

.INTERMEDIATE: $(patsubst %,node-init-genesis-server-cert-%,$(SERVER_CERTS))
node-init-genesis-server-cert-%:
	$(AKASH) tx cert generate server localhost akash-provider.localhost --from=$*
	$(AKASH) tx cert publish server --to-genesis=true --from=$*

.INTERMEDIATE: node-init-genesis-accounts
node-init-genesis-accounts: $(patsubst %,node-init-genesis-account-%,$(GENESIS_ACCOUNTS))
	$(AKASH) genesis validate

.INTERMEDIATE: $(patsubst %,node-init-genesis-account-%,$(GENESIS_ACCOUNTS))
node-init-genesis-account-%:
	$(AKASH) genesis add-account \
		"$(shell $(AKASH) $(KEY_OPTS) keys show "$(@:node-init-genesis-account-%=%)" -a)" \
		"$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

.INTERMEDIATE: node-init-gentx
node-init-gentx:
	$(AKASH) genesis gentx validator "$(CHAIN_VALIDATOR_DELEGATE)$(CHAIN_TOKEN_DENOM)" --min-self-delegation=1 --gas=auto --gas-prices=0.025uakt

.INTERMEDIATE: node-init-finalize
node-init-finalize:
	$(AKASH) genesis collect
	$(AKASH) genesis validate

.PHONY: node-run
node-run:
	$(AKASH) start --trace=true

.PHONY: node-status
node-status:
	$(AKASH) status

.PHONY: rest-server-run
rest-server-run:
	$(AKASH) rest-server

.PHONY: clean
clean: clean-$(AKASH_RUN_NAME)
	rm -rf "$(AKASH_RUN)/$(AKASH_RUN_NAME)"

.PHONY: rosetta-run
rosetta-run:
	$(AKASH) rosetta --addr localhost:8080 --grpc localhost:9090 --network=$(AKASH_CHAIN_ID) --blockchain=akash
