# AKT token metadata

Metadata is a mainnet launch requirement. Complete it before publishing the
canonical mint or opening pools. It gives wallets the token's name, symbol and
logo. It does not create a new mint, authorize issuance or grant a verification
badge.

## Contents

- [Mainnet values](#mainnet-values)
- [Contract and authority](#contract-and-authority)
- [Local setup](#local-setup)
- [Mainnet launch gate](#mainnet-launch-gate)

## Mainnet values

Use [`metadata/akt.json`](../metadata/akt.json) for mainnet:

- Name: **Akash Network Token**
- Symbol: **akt**
- Decimals: **6**, set on the mint
- Description: **AKT, the native token of Akash Network.**
- Website: **https://akash.network/**

The image is the existing Cosmos AKT logo from the
[Cosmos chain registry](https://github.com/cosmos/chain-registry/blob/f98e34339a1d30b4a2734b78d6dfcbba9cb02f8e/akash/images/akt.png),
an unchanged 800-by-800 PNG, copied into `akash-network/net`. The JSON pins
the image commit in that repository. Its SHA-256 is
`2f88bb209e166c871439b09c991046318d5fe0291a6f4b4711850d1f89725ce1`.

The approved mainnet JSON is hosted in [akash-network/net](https://github.com/akash-network/net/blob/3c0b4cce5fd3ab197ea6375ad3de6e8417580217/solana/mainnet/akt.json).
Pass its [raw JSON URL](https://raw.githubusercontent.com/akash-network/net/3c0b4cce5fd3ab197ea6375ad3de6e8417580217/solana/mainnet/akt.json)
to `set-token-metadata`. GitHub serves valid JSON as `text/plain; charset=utf-8`;
verify the parsed body and the image bytes. Publishing this file does not set
on-chain metadata. The token uses standard
SPL Token with Metaplex fungible metadata. Initializing the mint does not create
metadata.

## Contract and authority

`set_token_metadata(uri)` creates or updates the canonical Metaplex metadata
account for the existing AKT mint. It fixes the name to `Akash Network Token`, symbol
to `akt`, and royalties to zero. Metaplex records the token as fungible. The URI
must be HTTPS and fit its 200-byte field.

The current AKT program upgrade authority must sign. The mint-authority PDA
signs the CPI and remains the metadata update authority. It cannot sign outside
the program. The instruction passes the mint read-only and never invokes SPL
minting. Genesis, bridge proofs, balances, six decimals and the absent freeze
authority are unchanged. Metadata remains mutable under this authority.

The implementation uses Anchor SPL's Metaplex CPI wrapper and the official
Metaplex JavaScript account decoder. It introduces no custom metadata format.

## Local setup

Fresh validators preload the standard Token Metadata program at
`metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s`. Setup downloads its public
mainnet executable with a read-only `program dump`, checks its pinned SHA-256,
and caches it in ignored `../../.cache/solana/deps/metaplex/`. No private accounts, code, keys or
transactions are sent to that RPC.

```bash
npm run setup:metadata
npm test
```

The fixture checksum is
`31f0a627dba051a938de650464e55cc5397a4be0fd496929c1f9cf02fe5e9011`.
If upstream changes, setup fails instead of silently testing different code.
Retain a copy of the pinned binary for reproducibility, or review and
repin the replacement. This fixture is for local testing, not a mainnet program
deployment. Mainnet already has the Metaplex program.

On test networks the portal serves `/token/akt.json` with the same name, symbol
and logo URL. Its description explicitly marks it as a private testnet token,
and its website link points to the test portal. The JSON allows cross-origin reads. No IPFS upload,
public bucket, Metaplex registration or wallet-provider submission is needed.

After the updated mint program is deployed and genesis is initialized, use the
matching local profile, run name and **verified** genesis hash:

```bash
export AKT_LOCAL_PROFILE=testnetify AKT_LOCAL_RUN=dev
npm run local:metadata -- \
  --genesis YOUR_VERIFIED_LOCAL_GENESIS_HASH \
  --uri https://YOUR_PORTAL/token/akt.json
```

The operator checks ledger identity, executable Metaplex program, reachable
JSON and PNG, and on-chain metadata after finalized confirmation. Repeating
the command with matching metadata sends no transaction. It uses that run's
ignored local authority key. Mainnet uses a reviewed multisig transaction,
not this development-key command.

### Phantom

Phantom Localnet uses `127.0.0.1:8899`. Contract tests use separate ports
19899/19900 and stop their temporary validator afterward.

Receiving AKT, discovering its balance, loading its metadata, and showing its
logo are separate wallet operations. On-chain metadata does not guarantee a
Phantom badge, price feed, or logo. A wallet's external metadata proxy cannot
fetch a private test URL. Test the installed wallet after deployment;
do not expose the testnet to solve a wallet cache or proxy limitation.

## Mainnet launch gate

- Use the approved [mainnet values](#mainnet-values) and JSON. Do not publish
  the private-testnet portal's JSON as mainnet metadata.
- Host JSON and the logo at durable, publicly readable HTTPS URLs with no
  authentication. Check that the body parses as the approved JSON and that the
  image is served as `image/png`; GitHub raw JSON uses `text/plain`. No external
  approval or disclosure of source code or private keys is required. Use a
  versioned URI and retain the content/hash in the launch record.
- Confirm the production program, canonical mint, six decimals, absent freeze
  authority and mint-authority PDA. Bind the reviewed transaction to the correct
  cluster and canonical Metaplex program.
- Set the program upgrade authority to the approved multisig and time lock.
  Execute `set_token_metadata` through that authority. Record the transaction,
  metadata PDA, URI, update authority and chosen mutability policy. Permanently
  freezing metadata would need an explicitly reviewed instruction; this version
  keeps it mutable.
- Verify finalized metadata, the live JSON/logo, unchanged mint authority, and
  supply reconciliation. Check real funded wallets in Phantom, Solflare and the
  other supported wallets before calling display support complete.
- Publish the canonical mint on official Akash channels. Wallet verification,
  Jupiter and tracker listings are separate launch tasks; metadata does not
  automatically complete them.

References: [Metaplex's existing SPL token guide](https://www.metaplex.com/docs/smart-contracts/token-metadata/guides/how-to-add-metadata-to-spl-tokens)
and [Agave 2.3.13 validator source](https://github.com/anza-xyz/agave/blob/v2.3.13/validator/src/cli.rs).
