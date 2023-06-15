GON_CONFIGFILE           ?= gon.json

GORELEASER_SKIP_VALIDATE ?= false
GORELEASER_DEBUG         ?= false
GORELEASER_IMAGE         := ghcr.io/goreleaser/goreleaser-cross:v$(GOLANG_VERSION)
GORELEASER_RELEASE       ?= false
GORELEASER_MOUNT_CONFIG  ?= false

RELEASE_DOCKER_IMAGE     ?= ghcr.io/akash-network/node

ifeq ($(GORELEASER_RELEASE),true)
	GORELEASER_SKIP_VALIDATE := false
	GORELEASER_SKIP_PUBLISH  := release --skip-publish=false
else
	GORELEASER_SKIP_PUBLISH  := --skip-publish=true
	GORELEASER_SKIP_VALIDATE ?= false
	GITHUB_TOKEN=
endif

ifeq ($(GORELEASER_MOUNT_CONFIG),true)
	GORELEASER_IMAGE := -v $(HOME)/.docker/config.json:/root/.docker/config.json $(GORELEASER_IMAGE)
endif

ifeq ($(OS),Windows_NT)
	DETECTED_OS := Windows
else
	DETECTED_OS := $(shell sh -c 'uname 2>/dev/null || echo Unknown')
endif

# on MacOS disable deprecation warnings security framework
ifeq ($(DETECTED_OS), Darwin)
	export CGO_CFLAGS=-Wno-deprecated-declarations
endif

# if go.mod contains replace for any modules on local filesystem
# mount them into docker during goreleaser build to exactly same path
#REPLACED_MODULES              := $(shell go list -mod=readonly -m -f '{{ .Replace }}' all 2>/dev/null | grep -v -x -F "<nil>" | grep "^/")
#ifneq ($(REPLACED_MODULES), )
#	GORELEASER_MOUNT_REPLACED := $(foreach mod, $(REPLACED_MODULES), -v $(mod):$(mod)\\)
#endif
#GORELEASER_MOUNT_REPLACED     := $(GORELEASER_MOUNT_REPLACED:\\=)

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

.PHONY: docker-image
docker-image:
	docker run \
		--rm \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GO_MOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-v $(GOPATH):/go:ro \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(GO_MOD_NAME) \
		-w /go/src/$(GO_MOD_NAME) \
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
.PHONY: release
release: gen-changelog
	docker run \
		--rm \
		-e STABLE=$(IS_STABLE) \
		-e MOD="$(GO_MOD)" \
		-e BUILD_TAGS="$(BUILD_TAGS)" \
		-e BUILD_VARS="$(GORELEASER_BUILD_VARS)" \
		-e STRIP_FLAGS="$(GORELEASER_STRIP_FLAGS)" \
		-e LINKMODE="$(GO_LINKMODE)" \
		-e GITHUB_TOKEN="$(GITHUB_TOKEN)" \
		-e GORELEASER_CURRENT_TAG="$(RELEASE_TAG)" \
		-e DOCKER_IMAGE=$(RELEASE_DOCKER_IMAGE) \
		-e GOPATH=/go \
		-v $(GOPATH):/go:ro \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(GO_MOD_NAME) \
		-w /go/src/$(GO_MOD_NAME) \
		$(GORELEASER_IMAGE) \
		-f "$(GORELEASER_CONFIG)" \
		$(GORELEASER_SKIP_PUBLISH) \
		--skip-validate=$(GORELEASER_SKIP_VALIDATE) \
		--debug=$(GORELEASER_DEBUG) \
		--clean \
		--release-notes=/go/src/$(GO_MOD_NAME)/.cache/changelog.md
