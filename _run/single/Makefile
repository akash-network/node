include ../common.mk

KEY_OPTS 	= --keyring-backend test
akash 		= ../../akashctl --home $(CLIENT_HOME) 
akashd 		= ../../akashd --home $(NODE_HOME)

MINIKUBE_IP ?= $(shell minikube ip)

kube-start:
	minikube start --cpus 4 --memory 4096
	minikube addons enable ingress
	minikube addons enable metrics-server

init: init-keys init-genesis init-gentx init-collect-gentx

dirs: 
	mkdir -p $(CLIENT_HOME) $(NODE_HOME)

init-genesis: dirs
	$(akashd) init node0 --chain-id local
	$(akashd) add-genesis-account $(shell $(akash) $(KEY_OPTS) keys show validator -a) 100000000stake
	$(akashd) add-genesis-account $(shell $(akash) $(KEY_OPTS) keys show provider -a) 100000000stake

init-gentx:
	$(akashd) $(KEY_OPTS) gentx --name validator --home-client $(CLIENT_HOME)

init-collect-gentx:
	$(akashd) collect-gentxs
	$(akashd) validate-genesis

init-config: dirs
	$(akash) config chain-id local
	$(akash) config output json
	$(akash) config indent true
	$(akash) config trust-node true

init-keys: dirs
	$(akash) $(KEY_OPTS) keys add validator 
	$(akash) $(KEY_OPTS) keys add provider

deploy:
	 helm install node0 akash-node \
		 --set image.pullPolicy=Never \
		 --set "ingress.domain=$(MINIKUBE_IP).nip.io" \
		 --set-file node.genesis=$(NODE_HOME)/config/genesis.json \
		 --set-file node.priv_validator_key=$(NODE_HOME)/config/priv_validator_key.json \
		 --set-file node.priv_validator_state=$(NODE_HOME)/data/priv_validator_state.json \
		 --set-file node.node_key=$(NODE_HOME)/config/node_key.json \
		 --set-file node.app=$(NODE_HOME)/config/app.toml \
		 --set-file node.config=$(NODE_HOME)/config/config.toml

remove:
	helm uninstall node0

.PHONY: kube-start init dirs clean init-keys init-config init-collect-gentx init-gentx deploy remove
