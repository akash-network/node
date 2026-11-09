# Sink SDK integration

`chain-sdk` owns public types, generated clients, validation and CLI. `node/x/sink` owns storage codecs, native execution and the canonical proof fixtures; `contracts/solana` owns the Solana program and these services. The portal is built separately.

The TypeScript barrel exports `Sink_ID` and `Sink_Status`. Identity and all 64-bit values use bigint internally and decimal strings at JSON boundaries. Use `shared/id.ts` for parsing, canonical IDs and byte encodings; never convert identity fields to Number.

Canonical queries are `Transfer`, `Transfers`, `Disputes`, `Account`, `State`, `Params` and `Reporters`. REST uses `/akash/sink/v1/transfers/{owner}/{height}/{sequence}`, `/transfers`, `/disputes`, `/accounts/{owner}` and `/state`. `State` contains totals and epoch usage; `Params` contains configuration; `Reporters` version 0 returns the current policy, a positive version selects that policy, and maximum uint64 selects retained history. Read all three at one committed height. Reporter policy thresholds are decimal strings, not integer vote counts. Transfer history supports owner and financial-status filters with opaque server-issued pagination keys. Disputes include held terminal records. Use a fixed committed height throughout each page sequence; pruned state fails the request.

`MsgSinkResponse.id` contains the composite identity. Transaction-hash receipts bind the successful sole MsgSink, response ID, sender, block height, recipient and amount to the immutable record. ID receipts and owner history use canonical state queries and require no transaction index.

## Supply the locked package

Node's `go.mod` selects the SDK release. Build the TypeScript package from that
release's `go/<version>` tag; do not maintain a separate SDK commit pin. The SDK
repository contains generated TypeScript source, which its own npm build compiles
and packages. The services install the result from `.cache/solana/chain-sdk.tgz`;
`npm ci` verifies it against the committed `package-lock.json`.

Operator release CI does this automatically. To prepare the same input locally,
run from the node repository root with Node 24.22.1:

```bash
(
  set -euo pipefail
  umask 022
  cache="$PWD/.cache/solana"
  mkdir -p "$cache"
  sdk_dir=$(mktemp -d "$cache/sdk.XXXXXX")
  trap 'rm -rf "$sdk_dir"' EXIT
  version=$(GOWORK=off go list -mod=readonly -m -f '{{.Version}}' pkg.akt.dev/go)
  git clone --depth 1 --branch "go/$version" https://github.com/akash-network/chain-sdk.git "$sdk_dir"
  cd "$sdk_dir/ts"
  npm ci
  npm run build
  rm dist/tsconfig.build.tsbuildinfo
  archive=$(npm pack --ignore-scripts --silent --pack-destination "$cache")
  mv "$cache/$archive" "$cache/chain-sdk.tgz"
)
npm ci --prefix contracts/solana
```

The package omits TypeScript's machine-specific incremental-build cache. A
changed SDK pin may require a reviewed npm lockfile update. After building the
newly pinned release, update the package integrity deliberately:

```bash
cd contracts/solana
npm install ../../.cache/solana/chain-sdk.tgz
npm run typecheck
npm run test:services
```

The explicit package argument refreshes npm's same-version archive integrity in the lockfile. Install the same archive into the portal and compare installed package bytes in both consumers. Preserve version strings and unrelated lockfile fields. Service and program tests consume the native fixtures under `programs/akt/tests/fixtures`. The node generator owns the original record/settlement vectors; the Solana-owned boundary generator calls the unchanged node codecs and SDK encoders for wider addresses and the planned seven-reporter policy. Native `RecordData` and `SettlementData` values use generated encoders.

Run the real-chain fixture with `npm run test:sink` after node integration is available. See [canonical launch and recovery](sink-migration-v2.md) for separate deployment namespaces, durable state and validation gates. Historical evidence does not validate this schema.
