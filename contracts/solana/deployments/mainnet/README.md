# AKT mainnet artifacts

This directory preserves the original launch package. Mainnet is deployed.
The current AKT binary is in [the November 10 metadata upgrade](upgrades/20261110-token-metadata/README.md).
The light client and access manager still use the original binaries below.
Finalized bytes and Squads upgrade authorities were checked on November 10, 2026.

| Program                 | Current binary                                          | Program ID                                     |
| ----------------------- | ------------------------------------------------------- | ---------------------------------------------- |
| AKT bridge              | [akt.so](upgrades/20261110-token-metadata/build/akt.so) | `98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME` |
| Tendermint light client | [ics07_tendermint.so](build/ics07_tendermint.so)        | `2dTK4NwEkv3gyY5dZnG4TuDYFMoNYQYrBqEGwTSmEmKr` |
| Access manager          | [access_manager.so](build/access_manager.so)            | `HntLc7xrcRMRk9X38uMytCnweFJhVVCuzqhkVPe1RkCq` |

The canonical mint is `5d3fjPDdkYbtYcAMR9PC2TANaEyhzuyP8JxTEwqjVgBZ`.
All program upgrade authorities are the Squads vault
`H2sXJRVW3SnTdFGjTXpGSsVKN2HnDaKc6xLRomS36H3U`.
The [original manifest](manifest.json) records historical launch inputs, including
the retired ceremony address; it does not authorize that address today.

## Build record

- Original source: [`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`](https://github.com/akash-network/node/commit/431953bb96a3cfdd1e1888210cae6dbf7d05a4ad).
- Pinned IBC source: [`b3f25b45c785a9cb571cb169e5b612fb6c94e1f8`](https://github.com/akash-network/ibc-contracts/commit/b3f25b45c785a9cb571cb169e5b612fb6c94e1f8).
- Original interfaces and binaries: [build](build), [SHA256SUMS](build/SHA256SUMS), [build-info.txt](build/build-info.txt).
- Current AKT source, hash and unchanged-interface comparison: [metadata upgrade record](upgrades/20261110-token-metadata/README.md).

Each package keeps its original manifest and checksums. Do not replace the
historical AKT binary with the upgraded binary under the original manifest.
For installation, reproduction, upgrades and authority checks, use the single
[contract guide](../../docs/deploy-program.md).

## Historical checkpoint and rehearsal

[checkpoint-29447440.json](checkpoint-29447440.json) records the November 9
checkpoint agreed by three public RPC providers. It is historical initialization
evidence, not a current checkpoint or a file to reuse for client recovery.
The relayer advances the live client after initialization.

The exact-artifact local rehearsal used real Squads v4, two-of-five approvals
and no time lock. It exercised setup, zero initial supply, open header relaying,
atomic handover, rejection of the retired signer, activation, pause/resume,
proof-backed minting and replay rejection. It used synthetic validator signatures
and an explicitly trusted local checkpoint; it did not establish live service
readiness. The full [historical rehearsal report](https://github.com/akash-network/node/blob/a2f6ced18b46d39983a0f540143b18773a4c3492/contracts/solana/docs/mainnet-local-verification.md)
remains in Git history.

The pinned compiler emitted seven SBF stack-offset diagnostics, recorded in the
original evidence. This documentation update does not resolve them or claim an
independent second-machine build. Preserve that distinction when reviewing future
releases. Reporter and relayer releases are separate from these program artifacts.
