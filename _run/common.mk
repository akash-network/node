AKASH_ROOT := ../..

AKASH  := $(AKASH)/akash
AKASHD := $(AKASHD)/akashd

DATA_ROOT = data
NODE_ROOT = $(DATA_ROOT)/node

build:
	(cd $(AKASH_ROOT) && make build)
akash:
	(cd $(AKASH_ROOT) && make akash)
akashd:
	(cd $(AKASH_ROOT) && make akashd)
image-minikube:
	(cd $(AKASH_ROOT) && make image-minikube)

clean:
	rm -rf $(DATA_ROOT)
