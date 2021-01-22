GORELEASER_SKIP_VALIDATE ?= false

.PHONY: bins
bins: $(BINS)

.PHONY: build
build:
	$(GO) build ./...

.PHONY: akash
akash:
	$(GO) build $(BUILD_FLAGS) ./cmd/akash

.PHONY: akash_docgen
akash_docgen:
	$(GO) build -o akash_docgen $(BUILD_FLAGS) ./docgen

.PHONY: install
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

.PHONY: gen-changelog
gen-changelog: $(GIT_CHGLOG)
	@echo "generating changelog to .cache/changelog"
	./script/genchangelog.sh "$(GORELEASER_TAG)" .cache/changelog.md

.PHONY: release-dry-run
release-dry-run: modvendor gen-changelog
	docker run \
		--rm \
		--privileged \
		-e MAINNET=$(MAINNET) \
		-e BUILD_FLAGS="$(GORELEASER_FLAGS)" \
		-e LD_FLAGS="$(GORELEASER_LD_FLAGS)" \
		-e HOMEBREW_NAME="$(GORELEASER_HOMEBREW_NAME)" \
		-e HOMEBREW_CUSTOM="$(GORELEASER_HOMEBREW_CUSTOM)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		-f "$(GORELEASER_CONFIG)" --skip-validate=$(GORELEASER_SKIP_VALIDATE) --rm-dist --skip-publish --release-notes=/go/src/github.com/ovrclk/akash/.cache/changelog.md

.PHONY: release
release: modvendor gen-changelog
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
		-e HOMEBREW_NAME="$(GORELEASER_HOMEBREW_NAME)" \
		-e HOMEBREW_CUSTOM="$(GORELEASER_HOMEBREW_CUSTOM)" \
		--env-file .release-env \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		troian/golang-cross:${GOLANG_CROSS_VERSION} \
		-f "$(GORELEASER_CONFIG)" release --rm-dist --release-notes=/go/src/github.com/ovrclk/akash/.cache/changelog.md
