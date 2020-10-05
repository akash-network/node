# Golang modules and vendoring

.PHONY: deps-install
deps-install:
	$(GO) mod download

.PHONY: deps-tidy
deps-tidy:
	$(GO) mod tidy

.PHONY: deps-vendor
deps-vendor:
	go mod vendor

.PHONY: modsensure
modsensure: deps-tidy deps-vendor

.PHONY: modvendor
modvendor: modsensure $(MODVENDOR)
	@echo "vendoring non-go files files..."
	$(MODVENDOR) -copy="**/*.proto" -include=\
github.com/cosmos/cosmos-sdk/proto,\
github.com/tendermint/tendermint/proto,\
github.com/gogo/protobuf,\
github.com/regen-network/cosmos-proto/cosmos.proto
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=\
github.com/zondax/hid
