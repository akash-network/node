# AKT token metadata upgrade candidate

This build sets the existing Solana token's display name to **Akash** and
symbol to **AKT**. It has **not been deployed**. It replaces the earlier
uppercase-symbol-only candidate; use this package for the combined change.
Only `build/akt.so` needs an upgrade. The light client and access-manager
binaries are included for comparison.

## Scope and evidence

The complete compilation inputs were compared with deployed source
`431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`. Their only changes are the two
constants `TOKEN_NAME` and `TOKEN_SYMBOL` in `programs/akt/src/constants.rs`.
The accompanying source commits align metadata fixtures, expected values,
and documentation.

| Field  | Deployed value      | Upgrade value |
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

This candidate fits the existing program allocation. Both IBC binaries and
all six generated JSON/TypeScript interfaces are byte-for-byte identical to
the original release. The builder image and all recorded toolchain versions
also match. See [verification.json](verification.json) for source scope,
artifact hashes, and the public metadata check.

The pinned Docker build, 23 Rust unit tests, TypeScript check, and scoped
formatting/shell checks passed. The compiler emitted the same seven SBF
stack-offset diagnostics as the previous candidate; this change does not
address those upstream diagnostics. No blockchain simulation or upgrade
rehearsal was run for this candidate. Independent build comparison and human
upgrade review remain outstanding. The original launch rehearsal does not
establish that this upgrade has been executed or tested on-chain.

## Verify and reproduce

From the node repository root:

```bash
export UPGRADE_DIR="$PWD/contracts/solana/deployments/mainnet/upgrades/20261110-token-metadata"
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
export SOURCE=c815e3d9cc5dfcd4ebefc20dfd9ed7b3be3dc708
mkdir -p "$PWD/.cache/solana"
export OUT="$(mktemp -d "$PWD/.cache/solana/metadata-rebuild.XXXXXX")"
git fetch origin "$SOURCE"
bash contracts/solana/scripts/verifiable-build.sh --rev "$SOURCE" "$OUT"
diff "$UPGRADE_DIR/build/SHA256SUMS" "$OUT/SHA256SUMS"
diff "$UPGRADE_DIR/build/build-info.txt" "$OUT/build-info.txt"
```

The existing deployment policy requires two people to build on different
machines and compare results before deployment. This build is from one machine.

## Metadata and Squads execution order

[Net PR #719](https://github.com/akash-network/net/pull/719) capitalized the symbol;
[net PR #720](https://github.com/akash-network/net/pull/720) set the name to Akash.
Both are merged. The immutable URI for the combined metadata is:

```text
https://raw.githubusercontent.com/akash-network/net/130ca0ac6fab3bc1999e3612a1fbca83d57b439c/solana/mainnet/akt.json
```

The description, image, website, decimals, mint address, account layouts,
bridge policy, and service interfaces have no source changes. Publishing
these files does not alter the live program or metadata.

After review, use a funded upload payer to stage **only `build/akt.so`** in a
Solana buffer. Verify the buffer against this package's upgrade hash, then
transfer its authority to the Squads vault. The vault approves and executes
an upgrade of the existing AKT program through Squads. The light client and
access manager need no upgrade.

After that upgrade finalizes and the deployed bytes are verified, execute a
**separate Squads transaction** calling `set-token-metadata` with the new URI.
Upgrading alone does not rewrite the existing metadata account. Verify the
on-chain name, symbol, and operational state afterwards; wallet display caches
may refresh later. No relayer or reporter release is part of this change.
