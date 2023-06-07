# Golang modules and vendoring

.PHONY: deps-install
deps-install:
	go mod download

.PHONY: deps-tidy
deps-tidy:
	go mod tidy

.PHONY: deps-vendor
deps-vendor:
	go mod vendor

.PHONY: modsensure
modsensure: deps-tidy deps-vendor

.PHONY: modvendor
modvendor: $(MODVENDOR) modsensure
ifeq ($(GO_MOD), vendor)
modvendor:
	@echo "vendoring non-go files..."
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=github.com/zondax/hid
endif
