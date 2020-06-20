KIND_NAME      ?= $(shell basename $$PWD)
K8S_CONTEXT    ?= $(shell kubectl config current-context)
KIND_HTTP_PORT  = $(shell docker inspect \
										--type container "$(KIND_NAME)-control-plane" \
										--format '{{index .HostConfig.PortBindings "80/tcp" 0 "HostPort"}}')

PROVIDER_HOSTNAME ?= localhost
PROVIDER_HOST     ?= $(PROVIDER_HOSTNAME):$(KIND_HTTP_PORT)
PROVIDER_ENDPOINT ?= http://$(PROVIDER_HOST)

INGRESS_CONFIG_PATH ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml

kind-port:
	echo $(KIND_HTTP_PORT)
	echo $(KIND_PORT)

.PHONY: kind-cluster-create
kind-cluster-create:
	kind create cluster \
		--config kind-config.yaml \
		--name "$(KIND_NAME)"
	kubectl apply -f "$(INGRESS_CONFIG_PATH)"
	"$(AKASH_ROOT)/script/setup-kind.sh"

.PHONY: kind-cluster-delete
kind-cluster-delete:
	kind delete cluster --name "$(KIND_NAME)"

.PHONY: kind-cluster-clean
kind-cluster-clean:
	kubectl delete ns -l akash.network
