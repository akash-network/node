KIND_NAME      ?= $(shell basename $$PWD)
# 'kind' matches the configuration of the GH actions KinD. 
K8S_CONTEXT    ?= $(shell kubectl config current-context)
KIND_HTTP_PORT  ?= $(shell docker inspect \
										--type container "$(KIND_NAME)-control-plane" \
										--format '{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}')

KIND_HTTP_IP  ?= $(shell docker inspect \
										--type container "$(KIND_NAME)-control-plane" \
										--format '{{index .NetworkSettings.Ports "80/tcp" 0 "HostIp"}}')

KIND_K8S_IP ?= $(shell docker inspect \
										--type container "$(KIND_NAME)-control-plane" \
										--format '{{index .NetworkSettings.Ports "6443/tcp" 0 "HostIp"}}')

KIND_PORT_BINDINGS ?= $(shell docker inspect "$(KIND_NAME)-control-plane" \
										--format '{{index .NetworkSettings.Ports "80/tcp" 0 "HostPort"}}')

KIND_CONFIG       ?= kind-config.yaml

PROVIDER_HOSTNAME ?= localhost
PROVIDER_HOST     ?= $(PROVIDER_HOSTNAME):$(APP_HTTP_PORT)
PROVIDER_ENDPOINT ?= http://$(PROVIDER_HOST)

# TODO: referencing master/latest on a k8s repository is prone to breaking at some point.
INGRESS_CONFIG_PATH ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml

.PHONY: app-http-port
app-http-port:
	@echo $(KIND_HTTP_PORT)

.PHONY: kind-k8s-ip
kind-k8s-ip:
	@echo $(KIND_K8S_IP)

.PHONY: kind-port-bindings
kind-port-bindings:
	@echo $(KIND_PORT_BINDINGS)

.PHONY: kind-cluster-create
kind-cluster-create:
	kind create cluster \
		--config "$(KIND_CONFIG)" \
		--name "$(KIND_NAME)"
	kubectl apply -f "$(INGRESS_CONFIG_PATH)"
	"$(AKASH_ROOT)/script/setup-kind.sh"

.PHONY: kind-ingress-setup
kind-ingress-setup:
	kubectl apply -f "$(INGRESS_CONFIG_PATH)"
	"$(AKASH_ROOT)/script/setup-kind.sh"

.PHONY: kind-cluster-delete
kind-cluster-delete:
	kind delete cluster --name "$(KIND_NAME)"

.PHONY: kind-cluster-clean
kind-cluster-clean:
	kubectl delete ns -l akash.network
