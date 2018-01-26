all: $(patsubst %.dot,%.svg, $(wildcard *.dot))

%.svg: %.dot
	dot -Tsvg $< > $@
