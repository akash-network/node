# Sink SDK integration

`chain-sdk` owns public types, generated clients, validation and CLI. `node/x/sink` owns storage codecs, native execution and the canonical proof fixtures; `contracts/solana` owns the Solana program and these services. The portal is built separately.

The TypeScript barrel exports `Sink_ID` and `Sink_Status`. Identity and all 64-bit values use bigint internally and decimal strings at JSON boundaries. Use `shared/id.ts` for parsing, canonical IDs and byte encodings; never convert identity fields to Number.

Canonical queries are `Transfer`, `Transfers`, `Disputes`, `Account`, `State`, `Params` and `Reporters`. REST uses `/akash/sink/v1/transfers/{owner}/{height}/{sequence}`, `/transfers`, `/disputes`, `/accounts/{owner}` and `/state`. `State` contains totals and epoch usage; `Params` contains configuration; `Reporters` version 0 returns the current policy, a positive version selects that policy, and maximum uint64 selects retained history. Read all three at one committed height. Reporter policy thresholds are decimal strings, not integer vote counts. Transfer history supports owner and financial-status filters with opaque server-issued pagination keys. Disputes include held terminal records. Use a fixed committed height throughout each page sequence; pruned state fails the request.

`MsgSinkResponse.id` contains the composite identity. Transaction-hash receipts bind the successful sole MsgSink, response ID, sender, block height, recipient and amount to the immutable record. ID receipts and owner history use canonical state queries and require no transaction index.

## Supply the locked package

The paired inputs are node `b5c4affa65afb2645755645199492978ff6f4ce4` and chain-sdk `3b977f0785eef4a5cfdd06747a8084f2cd5161ba`. The SDK package is `1.0.0-alpha.46`, built from that exact committed source with its own `npm run build` pipeline and consumed as a reviewed `.tgz` input. Version alone does not identify this prerelease build. `npm ci`
verifies its integrity against `package-lock.json`; `npm run setup` never builds
or replaces it implicitly. Production packaging must receive that same archive.
A published version can replace this input once the matching SDK is released;
release/package publication is a separate task.

To update the dependency deliberately during SDK development, build the selected
SDK checkout first, then pack it and review the resulting lockfile change:

```bash
source scripts/workspace.sh
CHAIN_SDK_DIR=/absolute/path/to/reviewed/chain-sdk
npm ci --prefix "$CHAIN_SDK_DIR/ts"
npm run build --prefix "$CHAIN_SDK_DIR/ts"
sdk_package=$(cd "$CHAIN_SDK_DIR/ts" && npm pack --silent --pack-destination "$SOLANA_CACHE")
cp "$SOLANA_CACHE/$sdk_package" "$SOLANA_CACHE/chain-sdk.tgz"
npm install ../../.cache/solana/chain-sdk.tgz
npm run typecheck
npm run test:services
```

The explicit package argument refreshes npm's same-version archive integrity in the lockfile. Install the same archive into the portal and compare installed package bytes in both consumers. Preserve version strings and unrelated lockfile fields. Service and program tests consume the native fixtures under `programs/akt-solana/tests/fixtures`. The node generator owns the original record/settlement vectors; the Solana-owned boundary generator calls the unchanged node codecs and SDK encoders for wider addresses and the planned seven-reporter policy. Native `RecordData` and `SettlementData` values use generated encoders.

Run the real-chain fixture with `npm run test:sink` after node integration is available. See [canonical launch and recovery](sink-migration-v2.md) for separate deployment namespaces, durable state and validation gates. Historical evidence does not validate this schema.
