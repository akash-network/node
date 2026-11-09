# AKT mainnet program artifacts

Public build artifacts for the prepared AKT Solana mainnet launch. Deployment
and independent build approval remain pending. Private keys are not part of
this directory.

| Program                 | Binary                                           | Program ID                                     |
| ----------------------- | ------------------------------------------------ | ---------------------------------------------- |
| AKT mint                | [akt.so](build/akt.so)                           | `98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME` |
| Tendermint light client | [ics07_tendermint.so](build/ics07_tendermint.so) | `2dTK4NwEkv3gyY5dZnG4TuDYFMoNYQYrBqEGwTSmEmKr` |
| Access manager          | [access_manager.so](build/access_manager.so)     | `HntLc7xrcRMRk9X38uMytCnweFJhVVCuzqhkVPe1RkCq` |

The `build` directory also contains the generated JSON IDLs, TypeScript
interfaces, [SHA256SUMS](build/SHA256SUMS) and [build-info.txt](build/build-info.txt).
[manifest.json](manifest.json) binds their hashes to the launch program IDs,
ceremony public key, Squads vault and Solana mainnet genesis hash.

## Source and rebuild

- Node source: [`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`](https://github.com/akash-network/node/commit/431953bb96a3cfdd1e1888210cae6dbf7d05a4ad).
- IBC dependency: [`b3f25b45c785a9cb571cb169e5b612fb6c94e1f8`](https://github.com/akash-network/ibc-contracts/commit/b3f25b45c785a9cb571cb169e5b612fb6c94e1f8).

The artifact commit follows the source commit. `sourceCommit` identifies the
inputs to compilation; adding the resulting files does not change those inputs.
The launch mappings use `[programs.mainnet]` and the build scripts select that
cluster explicitly. Local test defaults remain localnet. Compiling sends no
transactions and needs no wallet or program keypairs.

From the node repository root, verify the checked-in files:

```bash
(cd contracts/solana/deployments/mainnet/build && shasum -a 256 -c SHA256SUMS)
```

With Docker running, Python 3.11 or newer, Bash and npm available, rebuild into
a new output directory:

```bash
export SOURCE=431953bb96a3cfdd1e1888210cae6dbf7d05a4ad
export OUT="$PWD/.cache/solana/reproduce-$SOURCE"
git fetch origin "$SOURCE"
(cd contracts/solana && npm run build:verifiable -- --rev "$SOURCE" "$OUT")
diff contracts/solana/deployments/mainnet/build/SHA256SUMS "$OUT/SHA256SUMS"
cat "$OUT/build-info.txt"
```

The script archives committed source and builds under fixed paths in the pinned
Linux AMD64 container, including on an Apple Silicon Mac. Cargo lockfiles are
enforced and regenerated IBC IDLs must match the committed interfaces. Full
toolchain versions and the builder image digest are in `build-info.txt`.
Check that the rebuilt source revisions and toolchain versions match that record.

At least two people must build on different machines and compare the results
before deployment. A second build on the same machine does not fulfill that
review. See the [reproducible build procedure](../../docs/deploy-program.md#3-reproducible-build-and-hashes).

## Verification

The clean build preserved the committed program declarations, Anchor configs
and Cargo locks. All six interfaces and both IBC binaries match the previous
launch build. AKT has a new binary hash, with the same program ID and size.

A fresh local rehearsal passed using these exact three binaries and the real
Squads v4 program: setup while paused, zero initial supply, explicit open
relaying, atomic handover, rejection of the retired signer, activation and
guardian pause. The local Squads used two of five members and a five-second
time lock; this does not test or change the production vault configuration.

The rehearsal fixture now uses the finalized validator clock, following the
existing local tests. Its first run used host time and was correctly rejected
by the consensus-timestamp check. The fixture correction changes no production
checks or compilation inputs; the deployment CLI matches the build source.

Rust unit tests, build/cache checks, deployment-tool tests, typechecking and
scoped shell/format checks passed. The pinned compiler still emits the same
seven SBF stack-offset diagnostics as the previous build. Those upstream
diagnostics are not resolved by this configuration change. Independent build
comparison and human launch review remain outstanding.

## Deployment

This directory contains only the on-chain contract build. Reporter, relayer and
header-helper releases are handled separately.

Use the [mainnet ceremony guide](../../docs/mainnet-ceremony.md). Copy this
directory to a separate release directory before using it for the ceremony;
transaction records and signing material must stay outside tracked source.
Use the CLI from the manifest's source commit and run its offline manifest
check before any transaction.

The launch starts with zero AKT supply, completes setup and the light-client
checkpoint while minting is paused, enables explicit open relaying, and then
hands control to the Squads vault. Initial AKT uses ordinary portal transfers.
The approved [token metadata](https://raw.githubusercontent.com/akash-network/net/84b94d96287d3507830a1d9832f55925f24b8dc8/solana/mainnet/akt.json)
is already published; it is set during launch setup.
