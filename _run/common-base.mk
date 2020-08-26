AKASH_ROOT   := ../..
AKASH_BIN   := $(AKASH_ROOT)/akash

.PHONY: all
all:
	(cd "$(AKASH_ROOT)" && make all)

.PHONY: bins
bins:
	(cd "$(AKASH_ROOT)" && make bins)

.PHONY: akash
akash:
	(cd "$(AKASH_ROOT)" && make akash)

.PHONY: image-minikube
image-minikube:
	(cd "$(AKASH_ROOT)" && make image-minikube)
