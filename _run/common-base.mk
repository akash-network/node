include $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/../make/init.mk)

AKASH_RUN_NAME := $(notdir $(CURDIR))

AKASH_HOME ?= $(AKASH_RUN)/$(AKASH_RUN_NAME)

.PHONY: all
all:
	(cd "$(AKASH_ROOT)" && make all)

.PHONY: bins
bins:
	(cd "$(AKASH_ROOT)" && make bins)

.PHONY: akash
akash:
	(cd "$(AKASH_ROOT)" && make)

.PHONY: image-minikube
image-minikube:
	(cd "$(AKASH_ROOT)" && make image-minikube)
