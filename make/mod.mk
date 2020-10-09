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

GOOGLE_API_PROTO_URL = https://raw.githubusercontent.com/googleapis/googleapis/master/google/api
GOOGLE_PROTO_TYPES   = $(CACHE_INCLUDE)/google/api

.PHONY: modvendor
modvendor: modsensure $(MODVENDOR)
	@echo "vendoring non-go files..."
	$(MODVENDOR) -copy="**/*.proto" -include=\
github.com/cosmos/cosmos-sdk/proto,\
github.com/cosmos/cosmos-sdk/third_party/proto
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=\
github.com/zondax/hid
