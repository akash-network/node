AP_RUN_DIR               := $(AKASH_RUN)/upgrade

AKASH_INIT               := $(AP_RUN_DIR)/.akash-init

export AKASH_HOME
export AKASH_KEYRING_BACKEND    = test
export AKASH_GAS_ADJUSTMENT     = 2
export AKASH_CHAIN_ID           = localakash
export AKASH_YES                = true
export AKASH_GAS_PRICES         = 0.025uakt
export AKASH_GAS                = auto
export AKASH_STATESYNC_ENABLE   = false
export AKASH_LOG_COLOR          = true

KEY_OPTS                := --keyring-backend=$(AKASH_KEYRING_BACKEND)
KEY_NAME                ?= validator
UPGRADE_TO              ?= $(shell $(ROOT_DIR)/script/upgrades.sh upgrade-from-release $(RELEASE_TAG))
UPGRADE_FROM            := $(shell cat $(ROOT_DIR)/meta.json | jq -r --arg name $(UPGRADE_TO) '.upgrades[$$name].from_version' | tr -d '\n')
GENESIS_BINARY_VERSION  := $(shell cat $(ROOT_DIR)/meta.json | jq -r --arg name $(UPGRADE_TO) '.upgrades[$$name].from_binary' | tr -d '\n')
UPGRADE_BINARY_VERSION  ?= local

REMOTE_TEST_WORKDIR     ?= ~/go/src/github.com/akash-network/node
REMOTE_TEST_HOST        ?=

$(AKASH_INIT):
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --gbv=$(GENESIS_BINARY_VERSION) --ufrom=$(UPGRADE_FROM) --uto=$(UPGRADE_TO) --config="$(PWD)/config.json" init
	touch $@

.PHONY: init
init: $(AKASH_INIT) $(COSMOVISOR)

.PHONY: genesis
genesis: $(GENESIS_DEST)

.PHONY: test
test: $(COSMOVISOR) init
	$(GO_TEST) -run "^\QTestUpgrade\E$$" -tags e2e.upgrade -timeout 180m -v -args \
		-cosmovisor=$(COSMOVISOR) \
		-workdir=$(AP_RUN_DIR)/validators \
		-config=test-config.json \
		-upgrade-name=$(UPGRADE_TO) \
		-upgrade-version="$(UPGRADE_BINARY_VERSION)" \
		-test-cases=test-cases.json

$(COSMOVISOR_DEBUG_VERSION_FILE): $(AKASH_DEVCACHE)
	@echo "installing cosmovisor for remote debug $(COSMOVISOR_VERSION) ..."
	rm -f $(COSMOVISOR_DEBUG)
	wget -qO- "https://github.com/cosmos/cosmos-sdk/releases/download/cosmovisor/$(COSMOVISOR_VERSION)/cosmovisor-$(COSMOVISOR_VERSION)-$(GOOS)-$(GOARCH).tar.gz" | \
		tar xvz -C $(AKASH_RUN_BIN) cosmovisor
	rm -rf "$(dir $@)"
	mkdir -p "$(dir $@)"
	touch $@
$(COSMOVISOR_DEBUG): $(COSMOVISOR_DEBUG_VERSION_FILE)

.PHONY: test-remote-prep
test-remote-prep: $(COSMOVISOR_DEBUG) bins
	$(GO_TEST) -c -tags e2e.upgrade -timeout 60m \
		-o $(AKASH_DEVCACHE_BIN)/upgrade.test \
		-gcflags "all=-N -l" pkg.akt.dev/node/tests/upgrade
	chmod +x $(AKASH_DEVCACHE_BIN)/upgrade.test
	rsync -Pl $(AKASH_DEVCACHE_BIN)/upgrade.test $(REMOTE_TEST_HOST):$(REMOTE_TEST_WORKDIR)/
	rsync -Prl --delete $(AKASH_RUN) $(REMOTE_TEST_HOST):$(REMOTE_TEST_WORKDIR)/
	rsync -Pl $(AKASH_ROOT)/tests/upgrade/test-config.json $(REMOTE_TEST_HOST):$(REMOTE_TEST_WORKDIR)
	rsync -Pl $(AKASH_ROOT)/tests/upgrade/test-cases.json $(REMOTE_TEST_HOST):$(REMOTE_TEST_WORKDIR)

.PHONY: test-remote-start
test-remote-start:
	ssh -t $(REMOTE_TEST_HOST) 'cd $(REMOTE_TEST_WORKDIR); bash -ic "\
		dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./upgrade.test \
		-- -test.v=test2json -test.paniconexit0 -test.run ^\QTestUpgrade\E$ -cosmovisor=$$(pwd)/bin/cosmovisor -workdir=$$(pwd)/run/upgrade/validators -config=test-config.json -upgrade-name=$(UPGRADE_TO) -upgrade-version=$(UPGRADE_BINARY_VERSION) -test-cases=test-cases.json"'

.PHONY: test-reset
test-reset:
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --config="$(PWD)/config.json" --uto=$(UPGRADE_TO) clean
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --config="$(PWD)/config.json" --uto=$(UPGRADE_TO) bins
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --config="$(PWD)/config.json" --uto=$(UPGRADE_TO) keys


.PHONY: bins
bins:
ifneq ($(findstring build,$(SKIP)),build)
bins:
	$(ROOT_DIR)/script/upgrades.sh --workdir=$(AP_RUN_DIR) --config="$(PWD)/config.json" --uto=$(UPGRADE_TO) bins
endif

.PHONY: clean
clean:
	rm -rf $(AP_RUN_DIR)
