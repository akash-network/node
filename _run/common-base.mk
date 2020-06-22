AKASH_ROOT   := ../..
AKASHCTL_BIN := $(AKASH_ROOT)/akashctl
AKASHD_BIN   := $(AKASH_ROOT)/akashd

.PHONY: all
all:
	(cd "$(AKASH_ROOT)" && make all)

.PHONY: bins
bins:
	(cd "$(AKASH_ROOT)" && make bins)

.PHONY: akashctl
akashctl:
	(cd "$(AKASH_ROOT)" && make akashctl)

.PHONY: akashd
akashd:
	(cd "$(AKASH_ROOT)" && make akashd)

.PHONY: image-minikube
image-minikube:
	(cd "$(AKASH_ROOT)" && make image-minikube)
