# Golang modules and vendoring

.PHONY: deps-install
deps-install:
	go mod download

.PHONY: deps-tidy
deps-tidy:
	go mod tidy
