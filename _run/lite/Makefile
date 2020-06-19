include ../common.mk

akashctl       = $(AKASHCTL) --home $(CLIENT_HOME) 
akashd         = $(AKASHD) --home $(NODE_HOME)
KEY_OPTS       = --keyring-backend=test

init: bins init-keys init-genesis init-gentx init-collect-gentx init-config

dirs: 
	mkdir -p "$(CLIENT_HOME)" "$(NODE_HOME)"

init-node: dirs
	$(akashd) init node0 $(CHAIN_OPTS)
	cp "$(GENESIS_PATH)" "$(GENESIS_PATH).orig"
	cat "$(GENESIS_PATH).orig" | \
		jq -rM '(..|objects|select(has("denom"))).denom           |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("bond_denom"))).bond_denom |= "$(CHAIN_TOKEN_DENOM)"' | \
		jq -rM '(..|objects|select(has("mint_denom"))).mint_denom |= "$(CHAIN_TOKEN_DENOM)"' > \
		"$(GENESIS_PATH)"

init-genesis: init-node
	$(akashd) add-genesis-account "$(shell $(akashctl) $(KEY_OPTS) keys show provider -a)"  "$(CHAIN_ACCOUNT_DEPOSIT)$(CHAIN_TOKEN_DENOM)"
	$(akashd) add-genesis-account "$(shell $(akashctl) $(KEY_OPTS) keys show validator -a)" "$(CHAIN_ACCOUNT_DEPOSIT)$(CHAIN_TOKEN_DENOM)"
	$(akashd) add-genesis-account "$(shell $(akashctl) $(KEY_OPTS) keys show main -a)"      "$(CHAIN_ACCOUNT_DEPOSIT)$(CHAIN_TOKEN_DENOM)"
	$(akashd) add-genesis-account "$(shell $(akashctl) $(KEY_OPTS) keys show other -a)"     "1$(CHAIN_TOKEN_DENOM)"
	$(akashd) validate-genesis

init-gentx:
	$(akashd) $(KEY_OPTS) gentx --name main --home-client "$(CLIENT_HOME)" --amount "$(CHAIN_MIN_DEPOSIT)$(CHAIN_TOKEN_DENOM)"

init-collect-gentx:
	$(akashd) collect-gentxs
	$(akashd) validate-genesis

init-config: dirs
	$(akashctl) config chain-id $(CHAIN_NAME)
	$(akashctl) config output json
	$(akashctl) config indent true
	$(akashctl) config trust-node true

init-keys: dirs
	$(akashctl) keys $(KEY_OPTS) add validator 
	$(akashctl) keys $(KEY_OPTS) add provider
	$(akashctl) keys $(KEY_OPTS) add main 
	$(akashctl) keys $(KEY_OPTS) add other

run-daemon:
	$(akashd) start

run-provider:
	$(akashctl) $(KEY_OPTS) --from provider provider run

MAIN_ADDR ?= $(shell $(akashctl) keys $(KEY_OPTS) show main -a)
PROV_ADDR ?= $(shell $(akashctl) keys $(KEY_OPTS) show provider -a)
query-status:
	$(akashctl) query account $(MAIN_ADDR)
	$(akashctl) query account $(PROV_ADDR)
	$(akashctl) query provider list
	$(akashctl) query market order list
	$(akashctl) query market bid list
	$(akashctl) query market lease list
	$(akashctl) query deployment list 

deploy:
	$(akashctl) tx $(KEY_OPTS) deployment $(CHAIN_OPTS) create deployment.yaml --from main -y

provider:
	$(akashctl) tx $(KEY_OPTS) provider $(CHAIN_OPTS) create provider.yaml --from provider -y

# variables to configure the sequenc IDs from as CLI parameters. 
# eg: make ... DSEQ=4412
DSEQ ?=	7
GSEQ ?=	5
OSEQ ?=	4
PRICE ?= 10akash
bid:
	$(akashctl) tx $(KEY_OPTS) $(CHAIN_OPTS) market  bid-create -y \
	--owner $(MAIN_ADDR) \
	--from provider \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  \
	--price "$(PRICE)"

bid-close: 
	$(akashctl) tx $(KEY_OPTS) $(CHAIN_OPTS) market bid-close -y \
	--owner $(MAIN_ADDR) \
	--from provider   \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  

order-close:
	$(akashctl) tx $(KEY_OPTS) $(CHAIN_OPTS) market order-close -y \
	--owner $(MAIN_ADDR) \
	--from main \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  

deploy-close:
	$(akashctl) tx $(KEY_OPTS) $(CHAIN_OPTS) deployment close -y \
	--owner "$(MAIN_ADDR)" \
	--from  main           \
	--dseq  "$(DSEQ)"

rest-server:
	$(akashctl) rest-server
