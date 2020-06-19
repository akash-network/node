AKASH_ROOT := ../..

AKASHCTL := $(AKASH_ROOT)/akashctl
AKASHD   := $(AKASH_ROOT)/akashd

DATA_ROOT 	= cache
NODE_HOME 	= $(DATA_ROOT)/node
CLIENT_HOME = $(DATA_ROOT)/client

CHAIN_NAME     = local
CHAIN_OPTS     = --chain-id $(CHAIN_NAME)
GENESIS_PATH   = $(NODE_HOME)/config/genesis.json

CHAIN_MIN_DEPOSIT     = 10000000
CHAIN_ACCOUNT_DEPOSIT = $(shell echo $$(($(CHAIN_MIN_DEPOSIT) * 10)))
CHAIN_TOKEN_DENOM     = akash

all:
	(cd $(AKASH_ROOT) && make all)

bins:
	(cd $(AKASH_ROOT) && make bins)

akashctl:
	(cd $(AKASH_ROOT) && make akashctl)

akashd:
	(cd $(AKASH_ROOT) && make akashd)

image-minikube:
	(cd $(AKASH_ROOT) && make image-minikube)

clean:
	rm -rf $(DATA_ROOT)

.PHONY: all build akash akashd image-minikube
