# KIND_NAME NOTE: 'kind' string literal is default for the GH actions 
# KinD, it's fine to use other names locally, however in GH container name 
# is configured by engineerd/setup-kind. `kind-control-plane` is the docker
# image's name in GH Actions.
KIND_NAME      ?= $(shell basename $$PWD)
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
KIND_CONFIG_CALICO ?= ../kind-config-calico.yaml

PROVIDER_HOSTNAME ?= localhost
PROVIDER_HOST     ?= $(PROVIDER_HOSTNAME):$(KIND_HTTP_PORT)
PROVIDER_ENDPOINT ?= http://$(PROVIDER_HOST)

# TODO: referencing master/latest on a k8s repository is prone to breaking at some point.
INGRESS_CONFIG_PATH ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
CALICO_MANIFEST ?= https://docs.projectcalico.org/v3.8/manifests/calico.yaml

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

.PHONY: kind-cluster-calico-create
kind-cluster-calico-create:
	kind create cluster \
		--config "$(KIND_CONFIG_CALICO)" \
		--name "$(KIND_NAME)"
	kubectl apply -f "$(CALICO_MANIFEST)"
	kubectl -n kube-system set env daemonset/calico-node FELIX_IGNORELOOSERPF=true
	# Calico needs to be managing networking before finishing setup
	kubectl apply -f "$(INGRESS_CONFIG_PATH)"
	$(AKASH_ROOT)/script/setup-kind.sh calico-metrics

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
