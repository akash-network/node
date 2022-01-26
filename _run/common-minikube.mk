INGRESS_CONFIG_PATH ?= ../ingress-nginx.yaml

MINIKUBE_VM_DRIVER ?=
MINIKUBE_IP         = $(shell minikube ip)
MINIKUBE_INVOKE     = VM_DRIVER=$(MINIKUBE_VM_DRIVER) ROOK_PATH=$(AKASH_ROOT)/_docs/rook/test $(AKASH_ROOT)/script/setup-minikube.sh

.PHONY: minikube-cluster-create
minikube-cluster-create: init-dirs
	$(MINIKUBE_INVOKE) up
	$(MINIKUBE_INVOKE) akash-setup
	kubectl apply -f ../ingress-nginx-class.yaml
	kubectl apply -f "$(INGRESS_CONFIG_PATH)"
	$(MINIKUBE_INVOKE) deploy-rook

.PHONY: minikube-cluster-delete
minikube-cluster-delete:
	$(MINIKUBE_INVOKE) clean

.PHONY: ip
ip:
	@echo $(MINIKUBE_IP)
