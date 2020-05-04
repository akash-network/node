AKASH_ROOT := ../..

AKASHCTL  := $(AKASHCTL)/akashctl
AKASHD := $(AKASHD)/akashd

DATA_ROOT 	= cache
NODE_HOME 	= $(DATA_ROOT)/node
CLIENT_HOME = $(DATA_ROOT)/client

all:
	(cd $(AKASH_ROOT) && make all)

build:
	(cd $(AKASH_ROOT) && make build)

akashctl:
	(cd $(AKASH_ROOT) && make akashctl)

akashd:
	(cd $(AKASH_ROOT) && make akashd)

image-minikube:
	(cd $(AKASH_ROOT) && make image-minikube)

clean:
	rm -rf $(DATA_ROOT)

.PHONY: all build akash akashd image-minikube
