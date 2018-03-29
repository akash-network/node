BINARY := run

DIRS := data/client data/provider

build:
	go build -o $(BINARY) .

init:
	mkdir -p $(DIRS)

run: build init
	./run eval

akash:
	(cd .. && make)

deps-install:
	go get github.com/buger/jsonparser
	go get github.com/ovrclk/gestalt/...

clean:
	rm -rf data $(BINARY)

reset: clean init build akash
rerun: reset run

.PHONY: build init run \
	akash deps-install \
	clean reset rerun
