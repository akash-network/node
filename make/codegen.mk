.PHONY: generate
generate:
	$(GO) generate ./...

.PHONY: mocks
mocks:
	mockery -case=underscore -dir provider              -output provider/mocks              -name StatusClient
	mockery -case=underscore -dir provider              -output provider/mocks              -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name ReadClient
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name Client
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name StatusClient

.PHONY: kubetypes
kubetypes: deps-vendor
	chmod +x vendor/k8s.io/code-generator/generate-groups.sh
	vendor/k8s.io/code-generator/generate-groups.sh all \
	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
	akash.network:v1

.PHONY: proto-gen
proto-gen: $(PROTOC) modvendor
	./script/protocgen.sh

.PHONY: codegen
codegen: generate proto-gen kubetypes mocks
