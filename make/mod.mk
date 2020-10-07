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
github.com/tendermint/tendermint/proto,\
github.com/gogo/protobuf,\
github.com/regen-network/cosmos-proto/cosmos.proto
	rm -rf $(GOOGLE_PROTO_TYPES)
	mkdir -p $(GOOGLE_PROTO_TYPES)
	curl -sSL $(GOOGLE_API_PROTO_URL)/http.proto > $(GOOGLE_PROTO_TYPES)/http.proto
	curl -sSL $(GOOGLE_API_PROTO_URL)/annotations.proto > $(GOOGLE_PROTO_TYPES)/annotations.proto
	curl -sSL $(GOOGLE_API_PROTO_URL)/httpbody.proto > $(GOOGLE_PROTO_TYPES)/httpbody.proto
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=\
github.com/zondax/hid
