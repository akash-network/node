include ../common.mk

akashctl       = $(AKASHCTL) --home $(CLIENT_HOME) 
akashd         = $(AKASHD) --home $(NODE_HOME)
KEY_OPTS       = --keyring-backend=test
CHAIN_NAME     = local
CHAIN_OPTS     = --chain-id $(CHAIN_NAME)

init: bins init-keys init-genesis init-gentx init-collect-gentx

dirs: 
	mkdir -p $(CLIENT_HOME) $(NODE_HOME)

init-genesis: dirs
	$(akashd) init node0 $(CHAIN_OPTS)
	$(akashd) add-genesis-account $(shell $(akashctl) $(KEY_OPTS) keys show provider -a) 100000akash,100000000stake
	$(akashd) add-genesis-account $(shell $(akashctl) $(KEY_OPTS) keys show validator -a) 100000akash,100000000stake
	$(akashd) add-genesis-account $(shell $(akashctl) $(KEY_OPTS) keys show main -a) 100000akash,100000000stake
	$(akashd) add-genesis-account $(shell $(akashctl) $(KEY_OPTS) keys show other -a) 1akash,1stake
	$(akashd) validate-genesis

init-gentx:
	$(akashd) $(KEY_OPTS) gentx --name main --home-client $(CLIENT_HOME)

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
	$(akashctl) tx $(KEY_OPTS) deployment $(CHAIN_OPTS) create deployment.yml --from main -y

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
