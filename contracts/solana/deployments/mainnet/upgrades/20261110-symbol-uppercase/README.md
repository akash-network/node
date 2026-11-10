# AKT uppercase-symbol upgrade candidate

This build changes the existing Solana AKT token symbol from `akt` to `AKT`.
It has **not been deployed**. Only `build/akt.so` is an upgrade candidate;
the light client and access-manager binaries are included for comparison.

## Scope and evidence

The complete compilation inputs were compared with deployed source
`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`. Their only change is
`TOKEN_SYMBOL` in `programs/akt/src/constants.rs`. The accompanying source
commit aligns metadata fixtures, expected values, and documentation.

- Build source: [`29b038b146377fbcab1d211ebef1439426645e6e`](https://github.com/akash-network/node/commit/29b038b146377fbcab1d211ebef1439426645e6e).
- IBC dependency: `b3f25b45c785a9cb571cb169e5b612fb6c94e1f8`, unchanged.
- AKT program: `98inUHb5mMaHWQZ9ut9PGrnmkMpncLZWWAyFwxCWagME`, unchanged.
- Mint: `5d3fjPDdkYbtYcAMR9PC2TANaEyhzuyP8JxTEwqjVgBZ`, unchanged.
- Squads upgrade authority: `H2sXJRVW3SnTdFGjTXpGSsVKN2HnDaKc6xLRomS36H3U`.

The new AKT binary has the same size, **1,040,040 bytes**. Exactly three bytes
change: `t` to `T` at file offset 390628, and `ak` to `AK` at offsets
390644–390645. Every other byte is identical to the original release.

| AKT binary | SHA-256                                                            |
| ---------- | ------------------------------------------------------------------ |
| Original   | `4cf85bb90cee9da1ebdaaed9921a9316d58d00d4142670b50419a5ca0ac22942` |
| Upgrade    | `6265e16194c84248cfc6439536f59952bba4c772bb0fdf0c3179c56df7c0f905` |

Both IBC binaries and all six generated JSON/TypeScript interfaces are
byte-for-byte identical to the original release. The builder image and all
recorded toolchain versions also match. See [verification.json](verification.json)
for the source scope, artifact hashes, exact byte differences, and metadata check.

The pinned Docker build, 23 Rust unit tests, TypeScript check, and scoped
formatting/shell checks passed. The compiler emitted seven SBF stack-offset
diagnostics, the same count documented for the original release; this change
does not address those upstream diagnostics. No blockchain simulation or upgrade
rehearsal was run for this candidate. Independent build comparison and human
upgrade review remain outstanding; the original launch rehearsal is not evidence
that this upgrade has been executed or tested on-chain.

## Verify and reproduce

From the node repository root:

```bash
export UPGRADE_DIR="$PWD/contracts/solana/deployments/mainnet/upgrades/20261110-symbol-uppercase"
(cd "$UPGRADE_DIR/build" && shasum -a 256 -c SHA256SUMS)
npm --prefix contracts/solana run deploy:programs -- check \
  --manifest "$UPGRADE_DIR/manifest.json"
```

The manifest uses the existing release-verification format. Its
`ceremonyAuthority` is the historical launch address, not an authorized upgrade
signer. This manifest is not a transaction proposal. Do not use the launch
`deploy_one` or `ceremony-execute` commands for this upgrade.

With Docker running and the prerequisites from the
[build procedure](../../../../docs/deploy-program.md#3-reproducible-build-and-hashes):

```bash
export SOURCE=29b038b146377fbcab1d211ebef1439426645e6e
mkdir -p "$PWD/.cache/solana"
export OUT="$(mktemp -d "$PWD/.cache/solana/uppercase-rebuild.XXXXXX")"
git fetch origin "$SOURCE"
bash contracts/solana/scripts/verifiable-build.sh --rev "$SOURCE" "$OUT"
diff "$UPGRADE_DIR/build/SHA256SUMS" "$OUT/SHA256SUMS"
diff "$UPGRADE_DIR/build/build-info.txt" "$OUT/build-info.txt"
```

The existing deployment policy requires two people to build on different
machines and compare results before deployment. The build above is from one
machine.

## Metadata and Squads execution order

[Net PR #719](https://github.com/akash-network/net/pull/719) changes only the
hosted JSON symbol. The new immutable URI is:

```text
https://raw.githubusercontent.com/akash-network/net/0603230f9c7a394ba48ac283208756e28deef42d/solana/mainnet/akt.json
```

The token name, description, image, website, decimals, mint address, account
layouts, bridge policy, and service interfaces have no source changes.
Publishing these files does not alter the live program or metadata.

After review, use a funded upload payer to stage **only `build/akt.so`** in a
Solana buffer. Verify the buffer against the upgrade hash above, then transfer
its authority to the Squads vault. The vault approves and executes an upgrade
of the existing AKT program through Squads. The light client and access manager
need no upgrade.

After that upgrade finalizes and the deployed bytes are verified, execute a
**separate Squads transaction** calling `set-token-metadata` with the new URI.
Upgrading alone does not rewrite the existing metadata account. Verify the
on-chain symbol and operational state afterwards; wallet display caches may
refresh later. No relayer or reporter release is part of this change.
