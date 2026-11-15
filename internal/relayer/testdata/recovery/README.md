# Legacy relayer recovery fixtures

These files were generated from baseline `14b2cd7042adb1f901ec44e0013e55284bbbe131` in this repository, using the actual implementations:

- `ts-intent.json`: `contracts/solana/local/transactions.ts` `sendTransaction`, web3.js signed v0 message. The mock connection asserts journal-before-send; the observer throws after settlement to retain the intent.
- `ts-proof-{transfer,settlement}.json`: `proof-job.ts` `newProofJob`, with actual cosmjs-types `MerkleProof` encoding and canonical key helpers. These are structurally valid synthetic two-level proof envelopes, not cryptographically verified chain membership proofs.
- `ts-ledger.json`: `relay-policy.ts` `SpendLedger`, including negative net spend, a charged signature, reclaimed liquidity, and a millisecond retry timestamp.
- `ts-discovery.json`: baseline v3 persisted cursor shape. Runtime cursor kind is restored as `tail`; it is not a persisted cursor field.
- `identity.json`: public test identities. The transfer sequence exceeds JavaScript's safe integer range and remains encoded in the canonical ID string.

Only deterministic test keys were used. Payer seed: SHA256 of the UTF-8 label `go-relayer-durable-test-payer-v1`. Genesis, program, and blockhash are SHA256 bytes of corresponding `go-relayer-durable-test-{genesis,program,blockhash}-v1` labels, encoded as base58 public keys. No private key file, real wallet, external RPC, or transaction broadcast was used. The in-process mock receives one signed send.

Generation called the baseline entry points above with installed Solana TypeScript development dependencies. The fixture JSON is the captured output; it is not synthesized by the Go tests. Production Go code does not load these dependencies.

`TestDurableLegacyTSReload` optionally runs the actual baseline TS decoders again. Set `RELAYER_TEST_TS_MODULES` to the absolute path of installed Solana TypeScript development dependencies (containing `tsx`, Anchor, and cosmjs-types). The final scoped acceptance run set this variable and passed without skips. The ordinary Go fixture tests do not require these modules; real lock interoperability tests require Node with TypeScript stripping and Python 3.

The JSON files are embedded in the test binary. `RELAYER_TEST_CACHE` may select an absolute writable test-only scratch root, including tmpfs inside a readonly image. It does not change production journal paths.
