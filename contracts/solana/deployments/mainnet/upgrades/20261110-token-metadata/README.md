# AKT token metadata upgrade record

This build sets the existing Solana token's display name to **Akash** and
symbol to **AKT**. It is deployed: finalized bytes and metadata were verified on November 10, 2026. It replaces the earlier
uppercase-symbol-only candidate; this package records the combined change.
Only `build/akt.so` needs an upgrade. The light client and access-manager
binaries are included for comparison.

## Scope and evidence

The complete compilation inputs were compared with deployed source
`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`. Their only changes are the two
constants `TOKEN_NAME` and `TOKEN_SYMBOL` in `programs/akt/src/constants.rs`.
The accompanying source commits align metadata fixtures, expected values,
and documentation.

| Field  | Original value      | Current value |
| ------ | ------------------- | ------------- |
| Name   | Akash Network Token | Akash         |
| Symbol | akt                 | AKT           |

- Build source: [`c815e3d9cc5dfcd4ebefc20dfd9ed7b3be3dc708`](https://github.com/akash-network/node/commit/c815e3d9cc5dfcd4ebefc20dfd9ed7b3be3dc708).
- IBC dependency: `b3f25b45c785a9cb571cb169e5b612fb6c94e1f8`, unchanged.
- AKT program: `98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME`, unchanged.
- Mint: `5d3fjPDdkYbtYcAMR9PC2TANaEyhzuyP8JxTEwqjVgBZ`, unchanged.
- Squads upgrade authority: `H2sXJRVW3SnTdFGjTXpGSsVKN2HnDaKc6xLRomS36H3U`.

| AKT binary | Bytes   | SHA-256                                                            |
| ---------- | ------- | ------------------------------------------------------------------ |
| Original   | 1040040 | `4cf85bb90cee9da1ebdaaed9921a9316d58d00d4142670b50419a5ca0ac22942` |
| Upgrade    | 1039968 | `d5dd4f833259b5f234461cd8af35e33092bc0e0015ac8097a9e7aeefd910c0ca` |

This build fits the existing program allocation. Both IBC binaries and
all six generated JSON/TypeScript interfaces are byte-for-byte identical to
the original release. The builder image and all recorded toolchain versions
also match. See [verification.json](verification.json) for source scope,
artifact hashes, and the public metadata check.

The pinned Docker build, 23 Rust unit tests, TypeScript check, and scoped
formatting/shell checks passed. The compiler emitted the same seven SBF
stack-offset diagnostics as the previous candidate; this change does not
address those upstream diagnostics. No blockchain simulation or upgrade
rehearsal was recorded in the original build evidence. Independent second-machine
comparison was not recorded there. The original launch rehearsal alone did not
establish execution of this upgrade; see the later finalized verification below.

## Verification and administration

Finalized AKT program bytes match the published upgrade hash over 1,039,968 bytes;
the remaining 72 bytes in its 1,040,040-byte allocation are zero. Both IBC programs
still match the original launch hashes. All three upgrade authorities are the
Squads vault. The on-chain metadata reads `Akash`, `AKT`, and this immutable URI:

```text
https://raw.githubusercontent.com/akash-network/net/130ca0ac6fab3bc1999e3612a1fbca83d57b439c/solana/mainnet/akt.json
```

[Net PR #719](https://github.com/akash-network/net/pull/719) and
[net PR #720](https://github.com/akash-network/net/pull/720) are merged.
The mint address, decimals, mint authority and account layouts are unchanged.
These live observations supplement the earlier build evidence; they do not
replace independent source review, build reproduction or a full refund rehearsal.

The [manifest](manifest.json), [checksums](build/SHA256SUMS),
[build provenance](build/build-info.txt) and [scope comparison](verification.json)
are immutable records. `ceremonyAuthority` in the manifest is historical, not a
current upgrade signer. Do not use launch commands to repeat this upgrade.

Use the [canonical contract guide](../../../../docs/deploy-program.md) for build
reproduction, buffer upload, Squads review, program verification and metadata
updates. This directory contains evidence, not a second deployment procedure.
