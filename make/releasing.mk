GON_CONFIGFILE           ?= gon.json

GORELEASER_VERBOSE       ?= false
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:$(GOTOOLCHAIN_SEMVER)
GORELEASER_RELEASE       ?= false
GORELEASER_MOUNT_CONFIG  ?= false
GORELEASER_SKIP          := $(subst $(COMMA),$(SPACE),$(GORELEASER_SKIP))

RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node

GORELEASER_MOD_MOUNT     ?= $(shell cat $(AKASH_ROOT)/.github/.repo | tr -d '\n')

ifneq ($(GORELEASER_RELEASE),true)
	ifeq (,$(findstring publish,$(GORELEASER_SKIP)))
		GORELEASER_SKIP += publish
	endif

	GITHUB_TOKEN=
endif

ifneq (,$(GORELEASER_SKIP))
	GORELEASER_SKIP := --skip=$(subst $(SPACE),$(COMMA),$(strip $(GORELEASER_SKIP)))
endif

ifeq ($(GORELEASER_MOUNT_CONFIG),true)
	GORELEASER_IMAGE := -v $(HOME)/.docker/config.json:/root/.docker/config.json $(GORELEASER_IMAGE)
endif

.PHONY: bins
bins: $(BINS)

.PHONY: build
build:
	$(GO_BUILD) -a  ./...

$(AKASH):
	$(GO_BUILD) -o $@ $(BUILD_FLAGS) ./cmd/akash

.PHONY: akash
akash: $(AKASH)

.PHONY: akash_docgen
akash_docgen: $(AKASH_DEVCACHE)
	$(GO_BUILD) -o $(AKASH_DEVCACHE_BIN)/akash_docgen $(BUILD_FLAGS) ./docgen

.PHONY: install
install:
	@echo installing akash
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

.PHONY: image-minikube
image-minikube:
	eval $$(minikube docker-env) && docker-image

.PHONY: test-bins
test-bins:
	docker run \
		--rm \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(GOPATH):/go \
		-v $(AKASH_ROOT):/go/src/$(GORELEASER_MOD_MOUNT) \
		-w /go/src/$(GORELEASER_MOD_MOUNT) \
		$(GORELEASER_IMAGE) \
		-f .goreleaser-test-bins.yaml \
		--verbose=$(GORELEASER_VERBOSE) \
		--clean \
		--skip=publish,validate \
		--snapshot

.PHONY: docker-image
docker-image:
	docker run \
		--rm \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(GOPATH):/go \
		-v $(AKASH_ROOT):/go/src/$(GORELEASER_MOD_MOUNT) \
		-w /go/src/$(GORELEASER_MOD_MOUNT) \
		$(GORELEASER_IMAGE) \
		-f .goreleaser-docker.yaml \
		--verbose=$(GORELEASER_VERBOSE) \
		--clean \
		--skip=publish,validate \
		--snapshot

.PHONY: gen-changelog
gen-changelog: $(GIT_CHGLOG)
	@echo "generating changelog to .cache/changelog"
	./script/genchangelog.sh "$(RELEASE_TAG)" .cache/changelog.md

.PHONY: release
release: gen-changelog
	docker run \
		--rm \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e GITHUB_TOKEN="$(GITHUB_TOKEN)" \
		-e GORELEASER_CURRENT_TAG="$(RELEASE_TAG)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-e GOPATH=/go \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(GOPATH):/go \
		-v $(AKASH_ROOT):/go/src/$(GORELEASER_MOD_MOUNT) \
		-w /go/src/$(GORELEASER_MOD_MOUNT) \
		$(GORELEASER_IMAGE) \
		-f "$(GORELEASER_CONFIG)" \
		release \
		$(GORELEASER_SKIP) \
		--verbose=$(GORELEASER_VERBOSE) \
		--clean \
		--release-notes=/go/src/$(GO_MOD_NAME)/.cache/changelog.md
