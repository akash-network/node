BINARY := run

build:
	go build -o $(BINARY) .

init:
	mkdir -p data/client

akash:
	(cd .. && make)

clean:
	rm -rf data $(BINARY)

reset: clean init build akash
