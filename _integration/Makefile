
build:
	go build -o main .

init:
	mkdir -p data/client

akash:
	(cd .. && make)

clean:
	rm -rf data

reset: clean init build akash
