.PHONY: generate
generate:
	$(GO) generate ./...

.PHONY: mocks
mocks:
	mockery -case=underscore -dir vendor/k8s.io/client-go/kubernetes -output testutil/kubernetes_mock -all -recursive -outpkg kubernetes_mocks -keeptree
	mockery -case=underscore -dir provider              -output provider/mocks              -name StatusClient
	mockery -case=underscore -dir provider              -output provider/mocks              -name Client
	mockery -case=underscore -dir provider/cluster/types      -output provider/cluster/mocks      -name Deployment
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Client
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name ReadClient
	mockery -case=underscore -dir provider/cluster      -output provider/cluster/mocks      -name Cluster
	mockery -case=underscore -dir provider/cluster/types      -output provider/cluster/mocks      -name Reservation
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name Client
	mockery -case=underscore -dir provider/manifest     -output provider/manifest/mocks     -name StatusClient
	mockery -case=underscore -dir client                -output client/mocks/               -name QueryClient
	mockery -case=underscore -dir client                -output client/mocks/               -name TxClient
	mockery -case=underscore -dir client                -output client/mocks/               -name Client


.PHONY: kubetypes
kubetypes: deps-vendor
	chmod +x vendor/k8s.io/code-generator/generate-groups.sh
	vendor/k8s.io/code-generator/generate-groups.sh all \
	github.com/ovrclk/akash/pkg/client github.com/ovrclk/akash/pkg/apis \
	akash.network:v1

.PHONY: proto-gen
proto-gen: $(PROTOC) $(GRPC_GATEWAY) $(PROTOC_GEN_COSMOS) modvendor proto-format
	./script/protocgen.sh

.PHONY: proto-swagger-gen
proto-swagger-gen: protoc-swagger modvendor
	./script/protoc-swagger-gen.sh

.PHONY: update-swagger-docs
update-swagger-docs: proto-swagger-gen
	statik -src=client/docs/swagger-ui -dest=client/docs -f -m
	if [ -n "$(git status --porcelain)" ]; then \
		echo "\033[91mSwagger docs are out of sync!!!\033[0m"; \
		exit 1; \
	else \
		echo "\033[92mSwagger docs are in sync\033[0m"; \
	fi

.PHONY: codegen
codegen: generate proto-gen update-swagger-docs kubetypes mocks
