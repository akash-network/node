GORELEASER_SKIP_VALIDATE ?= false
GORELEASER_DEBUG         ?= false
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:v$(GOLANG_VERSION)
GON_CONFIGFILE           ?= gon.json

ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
else
    DETECTED_OS := $(shell sh -c 'uname 2>/dev/null || echo Unknown')
endif

.PHONY: bins
bins: $(BINS)

.PHONY: build
build:
	$(GO) build -a  ./...

$(AKASH): modvendor
	$(GO) build -o $@ $(BUILD_FLAGS) ./cmd/akash

.PHONY: akash
akash: $(AKASH)

.PHONY: akash_docgen
akash_docgen: $(AKASH_DEVCACHE)
	$(GO) build -o $(AKASH_DEVCACHE_BIN)/akash_docgen $(BUILD_FLAGS) ./docgen

.PHONY: install
install:
	@echo installing akash
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

.PHONY: image-minikube
image-minikube:
	eval $$(minikube docker-env) && docker-image

.PHONY: docker-image
docker-image:
	docker run \
		--rm \
		--privileged \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GO_MOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		$(GORELEASER_IMAGE) \
		-f .goreleaser-docker.yaml \
		--debug=$(GORELEASER_DEBUG) \
		--rm-dist \
		--skip-validate \
		--skip-publish \
		--snapshot

.PHONY: gen-changelog
gen-changelog: $(GIT_CHGLOG)
	@echo "generating changelog to .cache/changelog"
	./script/genchangelog.sh "$(RELEASE_TAG)" .cache/changelog.md

.PHONY: release-dry-run
release-dry-run: modvendor gen-changelog
	docker run \
		--rm \
		--privileged \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GO_MOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e HOMEBREW_NAME="$(GORELEASER_HOMEBREW_NAME)" \
		-e HOMEBREW_CUSTOM="$(GORELEASER_HOMEBREW_CUSTOM)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		$(GORELEASER_IMAGE) \
		-f "$(GORELEASER_CONFIG)" \
		--skip-validate=$(GORELEASER_SKIP_VALIDATE) \
		--debug=$(GORELEASER_DEBUG) \
		--rm-dist \
		--skip-publish \
		--release-notes=/go/src/github.com/ovrclk/akash/.cache/changelog.md

.PHONY: release
release: modvendor gen-changelog
	@if [ ! -f ".release-env" ]; then \
		echo "\033[91m.release-env is required for release\033[0m";\
		exit 1;\
	fi
	docker run \
		--rm \
		--privileged \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GO_MOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e HOMEBREW_NAME="$(GORELEASER_HOMEBREW_NAME)" \
		-e HOMEBREW_CUSTOM="$(GORELEASER_HOMEBREW_CUSTOM)" \
		--env-file .release-env \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/github.com/ovrclk/akash \
		-w /go/src/github.com/ovrclk/akash \
		$(GORELEASER_IMAGE) \
		-f "$(GORELEASER_CONFIG)" release \
		--debug=$(GORELEASER_DEBUG) \
		--rm-dist \
		--release-notes=/go/src/github.com/ovrclk/akash/.cache/changelog.md
