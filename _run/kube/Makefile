include ../common.mk

akashctl       = $(AKASHCTL) --home $(CLIENT_HOME) 
akashd         = $(AKASHD) --home $(NODE_HOME)
LOCAL_DIR      = $(shell pwd)
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
	$(akashd) add-genesis-account $(shell $(akashctl) $(KEY_OPTS) keys show other -a) 10000akash,100stake
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
	$(akashctl) $(KEY_OPTS) --from provider  provider run

minikube:
	minikube start --cpus 2 --memory 3064

status:
	$(akashctl) status

NAME ?= "nil"
ACCT_ADDR ?= $(shell $(akashctl) keys show $(NAME) $(KEY_OPTS) -a)
query-account:
	$(akashctl) query account $(ACCT_ADDR)

query-acct-nonce:
	$(akashctl) query account

MAIN_ADDR ?= $(shell $(akashctl) keys $(KEY_OPTS) show main -a)
PROV_ADDR ?= $(shell $(akashctl) keys $(KEY_OPTS) show provider -a)
OTHR_ADDR ?= $(shell $(akashctl) keys $(KEY_OPTS) show other -a)
query-status:
	$(akashctl) query account $(MAIN_ADDR) # main
	$(akashctl) query account $(OTHR_ADDR) # other
	$(akashctl) query account $(PROV_ADDR) # provider
	$(akashctl) query provider list
	$(akashctl) query market order list
	$(akashctl) query market bid list
	$(akashctl) query market lease list
	$(akashctl) query deployment list 

query-market:
	$(akashctl) query market order list
	$(akashctl) query market bid list
	$(akashctl) query market lease list

# eg: TXN=37E016553DFC9305E8B56D5A8A9EA7E00B9CD6BF860055F7EDC1A2CE3ABCC6EE make query-txn
TXN ?=	"nil"
query-txn:
	$(akashctl) query tx $(TXN) $(CHAIN_OPTS) 

send-to-main:
	$(akashctl) tx send $(OTHR_ADDR) $(MAIN_ADDR) 117akash $(KEY_OPTS) $(CHAIN_OPTS) --memo "send monies to main" -y --fees 10akash --trace

deploy:
	$(akashctl) tx  deployment create deployment.yml --from main $(CHAIN_OPTS) $(KEY_OPTS) -y

provider-create:
	$(akashctl) tx $(KEY_OPTS) provider $(CHAIN_OPTS) create provider.yml --from provider -y

provider-get:
	$(AKASHCTL) query provider get $(PROV_ADDR) $(CHAIN_OPTS)

# variables to configure the sequenc IDs from as CLI parameters. 
# eg: make ... DSEQ=4412
DSEQ ?=	100
GSEQ ?= 1
OSEQ ?= 1
PRICE ?= 10akash
bid:
	$(akashctl) tx  market bid-create -y \
	$(KEY_OPTS) $(CHAIN_OPTS) \
	--owner $(MAIN_ADDR) \
	--from provider \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  \
	--price "$(PRICE)"

bid-close: 
	$(akashctl) tx market bid-close -y \
	$(KEY_OPTS) $(CHAIN_OPTS) \
	--owner $(MAIN_ADDR) \
	--from provider   \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  

order-close:
	$(akashctl) tx market order-close -y \
	$(KEY_OPTS) $(CHAIN_OPTS) \
	--owner $(MAIN_ADDR) \
	--from main \
	--dseq "$(DSEQ)"  \
	--gseq "$(GSEQ)"  \
	--oseq "$(OSEQ)"  

deploy-close:
	$(akashctl) tx deployment close -y \
	$(KEY_OPTS) $(CHAIN_OPTS) \
	--owner "$(MAIN_ADDR)" \
	--from  main           \
	--dseq  "$(DSEQ)"

rest-server:
	$(akashctl) rest-server

MINIKUBE_IP ?= $(shell minikube ip)
ping:
	curl -I "hello.$(MINIKUBE_IP).nip.io"
