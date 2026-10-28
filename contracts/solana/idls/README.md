# Upstream interfaces

These IDLs are generated from `cosmos/ibc-contracts` at commit `dc78ade828b766b218fb6bcb5417fdaf8e553021`. The Tendermint IDL includes the additive instructions, account and error codes from [`../patches/ics07-signature-batches.patch`](../patches/ics07-signature-batches.patch) and [`../patches/ics07-consensus-pruning.patch`](../patches/ics07-consensus-pruning.patch); earlier entries are unchanged. The monotonic latest height and open relaying patches change no interface. The access-manager IDL is unchanged. See [verifier patches](../docs/verifier-patches.md).

- `ibc-solana/programs/ics07-tendermint`, Apache-2.0.
- `ibc-solana/programs/access-manager`, Apache-2.0.

`npm run setup:light-client` downloads that revision, applies the checked-in patches, builds the two programs and compares their generated IDLs with these files. The AKT program uses Anchor's `declare_program!`, in its own crate `programs/akt-solana/ics07-tendermint-interface`, to generate its membership CPI and account decoding from the Tendermint IDL; the separate crate keeps the verifier's errors out of AKT's IDL. No signature or Merkle verification code is copied into AKT.

The IDL's program address is used by local tests. The bridge stores its authorized deployment address once during configuration and validates account owners and PDAs against that address. A production deployment may use its own program ID with the same ABI. Its binary must be built for that ID and reviewed separately.

Source: <https://github.com/cosmos/ibc-contracts/tree/dc78ade828b766b218fb6bcb5417fdaf8e553021/ibc-solana>
