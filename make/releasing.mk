GON_CONFIGFILE           ?= gon.json

GORELEASER_VERBOSE       ?= false
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:$(GOTOOLCHAIN_SEMVER)
GORELEASER_RELEASE       ?= false
GORELEASER_MOUNT_CONFIG  ?= false
GORELEASER_SKIP          := $(subst $(COMMA),$(SPACE),$(GORELEASER_SKIP))
RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node
#GORELEASER_MOD_MOUNT     ?= $(shell git config --get remote.origin.url | sed -r 's/.*(\@|\/\/)(.*)(\:|\/)([^:\/]*)\/([^\/\.]*)\.git/\2\/\4\/\5/' | tr -d '\n')
GORELEASER_MOD_MOUNT     ?= $(shell cat $(ROOT_DIR)/.github/repo | tr -d '\n')

RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node

GORELEASER_GOWORK        := $(GOWORK)

ifneq ($(GOWORK), off)
	GORELEASER_GOWORK    := /go/src/$(GORELEASER_MOD_MOUNT)/go.work
endif

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
	$(GO_BUILD) -a $(BUILD_FLAGS) ./...

.PHONY: $(AKASH)
$(AKASH): wasmvm-libs
	$(GO_BUILD) -v -o $@ $(BUILD_FLAGS) ./cmd/akash

.PHONY: akash
akash: $(AKASH)

.PHONY: akash_docgen
akash_docgen: $(AKASH_DEVCACHE)
	$(GO_BUILD) $(BUILD_FLAGS) -o $(AKASH_DEVCACHE_BIN)/akash_docgen ./docgen

.PHONY: install
install:
	@echo installing akash
	$(GO) install $(BUILD_FLAGS) ./cmd/akash

.PHONY: image-minikube
image-minikube:
	eval $$(minikube docker-env) && docker-image

.PHONY: test-bins
test-bins: wasmvm-libs
	docker run \
		--rm \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(GORELEASER_TAGS)" \
		-e BUILD_LDFLAGS="$(GORELEASER_LDFLAGS)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-e GOWORK="$(GORELEASER_GOWORK)" \
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
docker-image: wasmvm-libs
	docker run \
		--rm \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(GORELEASER_TAGS)" \
		-e BUILD_LDFLAGS="$(GORELEASER_LDFLAGS)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-e GOWORK="$(GORELEASER_GOWORK)" \
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
release: wasmvm-libs gen-changelog
	docker run \
		--rm \
		-e MOD="$(GOMOD)" \
		-e BUILD_TAGS="$(GORELEASER_TAGS)" \
		-e BUILD_LDFLAGS="$(GORELEASER_LDFLAGS)" \
		-e GITHUB_TOKEN="$(GITHUB_TOKEN)" \
		-e GORELEASER_CURRENT_TAG="$(RELEASE_TAG)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOTOOLCHAIN="$(GOTOOLCHAIN)" \
		-e GOWORK="$(GORELEASER_GOWORK)" \
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
		--release-notes=/go/src/$(GORELEASER_MOD_MOUNT)/.cache/changelog.md
