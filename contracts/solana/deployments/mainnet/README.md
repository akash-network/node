# AKT mainnet program artifacts

The token metadata upgrade candidate is in
[upgrades/20261110-token-metadata](upgrades/20261110-token-metadata/README.md).
The original launch artifacts below are preserved.

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

[checkpoint-29447440.json](checkpoint-29447440.json) is the public post-upgrade
Akash checkpoint agreed by three RPC providers on November 9, 2026. It is runtime
launch input, separate from the compiled build. The [command guide](../../docs/mainnet-launch-commands.md#5-set-the-checkpoint-and-bridge-limits--read-prepare-review-write)
records its comparison procedure, selected trust settings and expiry.

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

The [complete local contract rehearsal](../../docs/mainnet-local-verification.md)
passed using these exact three binaries and the real Squads v4 program with
2-of-5 approvals and zero-second time lock. It verified setup while paused,
zero initial supply, explicit open relay by an unlisted signer, atomic handover,
retired-signer rejection, activation, pause/resume, a real Akash app proof mint,
altered-amount rejection and replay rejection. The final state was paused.
An earlier five-second rehearsal also checked rejection before its time lock
expired. Neither run changes or proves control of the production vault.

The accepted extended rehearsal uses synthetic local validator signatures and
an explicitly trusted checkpoint. Its evidence and test-harness corrections are
recorded in the linked report. Production code and compilation inputs remain
unchanged; the deployment CLI matches the build source.

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
