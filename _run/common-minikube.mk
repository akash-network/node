INGRESS_CONFIG_PATH ?= ../ingress-nginx.yaml

MINIKUBE_VM_DRIVER ?=
MINIKUBE_IP         = $(shell VM_DRIVER=$(MINIKUBE_VM_DRIVER) $(AKASH_ROOT)/script/setup-minikube.sh ip)
MINIKUBE_INVOKE     = VM_DRIVER=$(MINIKUBE_VM_DRIVER) ROOK_PATH=$(AKASH_ROOT)/_docs/rook $(AKASH_ROOT)/script/setup-minikube.sh

.PHONY: minikube-cluster-create
minikube-cluster-create: init-dirs
	$(MINIKUBE_INVOKE) up
	$(MINIKUBE_INVOKE) akash-setup
	$(MINIKUBE_INVOKE) deploy-rook

.PHONY: minikube-cluster-delete
minikube-cluster-delete:
	$(MINIKUBE_INVOKE) clean

.PHONY: ip
ip:
	@echo $(MINIKUBE_IP)
