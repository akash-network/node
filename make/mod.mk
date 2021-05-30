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
modvendor: $(MODVENDOR) modsensure
	@echo "vendoring non-go files..."
	$(MODVENDOR) -copy="**/*.proto" -include=\
github.com/cosmos/cosmos-sdk/proto,\
github.com/cosmos/cosmos-sdk/third_party/proto
	$(MODVENDOR) -copy="**/*.h **/*.c" -include=\
github.com/zondax/hid
	$(MODVENDOR) -copy="**/swagger.yaml" -include=\
github.com/cosmos/cosmos-sdk/client/docs
	$(MODVENDOR) -copy="**/*.go.txt **/*.sh" -include=\
k8s.io/code-generator
