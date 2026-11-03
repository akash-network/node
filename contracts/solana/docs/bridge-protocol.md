# Canonical Sink bridge protocol

The authoritative proof and settlement reference is [launch and recovery](sink-migration-v2.md). This deployment requires a fresh chain, program and mint.

`mint_transfer(id, recipient_key, amount, proof_height)` reconstructs native protobuf `RecordData` from a composite Anchor `TransferId`, recipient public key and positive u64 amount. `void_transfer(id, settlement_status, proof_height)` proves the complete canonical settlement with its permanent void authorization. Both create the same permanent status PDA. The actual proven key determines the full 32-byte SHA-256 proof-buffer nonce. Identity is authenticated by the canonical proof key.

The mint instruction verifies the client pin, active trusting period, proof height and real ICS23 membership before validating recipient eligibility. Mint outcomes record recipient and amount. Void outcomes record invalid-recipient or governance reason. Both status and event record full identity, proof height and resolved slot. Governance void validates the permanent authorization in the full proven settlement value under its current status key. The request height and round must fit that snapshot.

Only top-level mint and void instructions are accepted. Only compute-budget instructions may precede them. Recipient checks, ATA ownership, vault curve exceptions, mint authority, daily and individual amount caps, payer-owned proof buffers and transaction rollback remain mandatory. Pause and cap failures create no permanent status. Invalid recipients can resolve to void, never to a deadline refund.

Repinning a light client does not change transfer identity or replay protection. Status accounts are permanent and cannot be closed. A mint prevents a later void and a void prevents a later mint.

`tests/bridge.ts` creates real IAVL and multistore ICS23 proofs and initializes explicit trusted roots on a fresh validator. `tests/sink-proof.ts` consumes the actual signed-deposit app fixture and exports finalized reporter transactions. The first suite tests membership and program execution. The second also tests chain-produced proof bytes. Neither substitutes for signed-header verification, which uses the relayer/header scenarios.
