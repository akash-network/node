# Sink SDK integration

`chain-sdk` owns the canonical schema 4, generated Go and TypeScript clients, validation, CLI and proof vectors. `node/x/sink` owns native execution; `contracts/solana` owns the Solana program and these services. The portal is built separately.

The TypeScript barrel exports `Sink_ID` and `Sink_Status`. Identity and all 64-bit values use bigint internally and decimal strings at JSON boundaries. Use `shared/id.ts` for parsing, canonical IDs and byte encodings; never convert identity fields to Number.

Canonical queries are `Transfer`, `Transfers`, `Disputes`, `Account` and `State`. REST uses `/akash/sink/v1/transfers/{owner}/{height}/{sequence}`, `/transfers`, `/disputes`, `/accounts/{owner}` and `/state`. Transfer history supports owner and financial-status filters with native pagination keys. Disputes include held terminal records. Use a fixed committed height throughout each page sequence; pruned state fails the request.

`MsgSinkResponse.id` contains the composite identity. Transaction-hash receipts bind the successful sole MsgSink, response ID, sender, block height, recipient and amount to the immutable record. ID receipts and owner history use canonical state queries and require no transaction index.

## Refresh the local package

Wait for the SDK worker to finish before building its checkout. `npm run setup` builds and packs that SDK into the node repository's `.cache/solana/chain-sdk.tgz`. To use an already built SDK:

```bash
source scripts/workspace.sh
sdk_package=$(cd "$CHAIN_SDK_DIR/ts" && npm pack --silent --pack-destination "$SOLANA_CACHE")
cp "$SOLANA_CACHE/$sdk_package" "$SOLANA_CACHE/chain-sdk.tgz"
npm install --force ../../.cache/solana/chain-sdk.tgz
npm run typecheck
npm run test:services
```

The explicit package argument refreshes npm's same-version archive integrity in the lockfile. Install the same archive into the portal and compare installed package bytes in both consumers. Preserve version strings and unrelated lockfile fields. Service proof tests independently consume `chain-sdk/go/node/sink/v1/testdata/proof_vectors.json` and `settlement_vectors.json`. Program fixtures come from the same SDK generator. Native `RecordData` and `SettlementData` values use generated encoders.

Run the real-chain fixture with `npm run test:sink` after node integration is available. See [canonical launch and recovery](sink-migration-v2.md) for separate deployment namespaces, durable state and validation gates. Historical evidence does not validate this schema.
