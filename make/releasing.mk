.PHONY: bins
bins: $(BINS)

.PHONY: build
build:
	$(GO) build ./...

akash:
	$(GO) build $(BUILD_FLAGS) ./cmd/akash

install:
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

.PHONY: image-minikube
image-minikube:
	eval $$(minikube docker-env) && docker-image

.PHONY: docker-image
docker-image:
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		-e GOLANG_VERSION="$(GOLANG_VERSION)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION}-linux-amd64 \
		-f .goreleaser-docker.yaml --rm-dist --skip-validate --skip-publish --snapshot

.PHONY: release-dry-run
release-dry-run: modvendor
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		-f .goreleaser.yaml --rm-dist --skip-validate --skip-publish

.PHONY: release
release: modvendor
	@if [ ! -f ".release-env" ]; then \
		echo "\033[91m.release-env is required for release\033[0m";\
		exit 1;\
	fi
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		--env-file .release-env \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		-f .goreleaser.yaml release --rm-dist
