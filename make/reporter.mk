AKT_REPORTER      := $(AKASH_DEVCACHE_BIN)/akt-reporter
AKT_REPORTER_PKGS := ./operator/... ./cmd/akt-reporter/...

# Set GORELEASER to pin the binary, for example $(AKASH_DEVCACHE)/tools/bin/goreleaser (CI uses v2.17.1).
GORELEASER        ?= goreleaser

.PHONY: akt-reporter
akt-reporter: $(AKASH_DEVCACHE)
	CGO_ENABLED=0 GOWORK=off $(GO_BUILD) -trimpath -o $(AKT_REPORTER) ./cmd/akt-reporter

.PHONY: test-reporter
test-reporter:
	GOWORK=off $(GO_VET) $(AKT_REPORTER_PKGS)
	GOWORK=off CGO_ENABLED=1 $(GO_TEST) -race $(AKT_REPORTER_PKGS)

# The repository's tags are prefixed (relayer/v*, reporter/v*), which GoReleaser cannot read as a version.
# REPORTER_TAG names the version of the snapshot archives and images, as GORELEASER_CURRENT_TAG does in a release.
REPORTER_TAG      ?= v0.0.0

.PHONY: reporter-snapshot
reporter-snapshot:
	GORELEASER_CURRENT_TAG=$(REPORTER_TAG) $(GORELEASER) release --snapshot --clean -f .goreleaser-reporter.yaml

# Needs the contracts/solana development setup; see contracts/solana/docs/reporter.md.
.PHONY: reporter-goldens
reporter-goldens:
	cd contracts/solana && npx tsx reporter/goldens.ts
