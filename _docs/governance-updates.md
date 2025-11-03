# Contract Governance Updates

Both the Wormhole and Pyth contracts are instantiated with the governance module address
(`akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f`) as their admin. This document describes
how to update contract parameters post-deployment.

## Updating Pyth Price Feed ID

The Pyth contract stores the AKT/USD price feed ID in its own state. To update it, submit a
governance proposal that executes `UpdateConfig` on the contract:

```json
{
  "messages": [
    {
      "@type": "/cosmwasm.wasm.v1.MsgExecuteContract",
      "sender": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
      "contract": "<pyth-contract-address>",
      "msg": "{\"update_config\":{\"price_feed_id\":\"0x<new-feed-id>\"}}",
      "funds": []
    }
  ],
  "deposit": "100000000uakt",
  "title": "Update AKT Price Feed ID",
  "summary": "Update the Pyth price feed ID to <new-id>"
}
```

The `UpdateConfig` message also accepts optional `wormhole_contract` and `data_sources` fields
to update those values in the same proposal.

```bash
# Submit the proposal
akash tx gov submit-proposal update-feed-id.json \
  --from <proposer-key> \
  --chain-id akashnet-2

# Vote
akash tx gov vote <proposal-id> yes --from <validator-key>
```

## Updating Wormhole Guardian Set

The Wormhole contract stores guardian sets internally. Updates happen via **Wormhole governance
VAAs** — messages signed by 2/3+1 of the current guardian set that contain the new guardian
addresses (action type 2).

Any account can submit a valid governance VAA; no Akash governance proposal is required:

```bash
akash tx wasm execute <wormhole-contract-address> \
  '{"submit_vaa":{"vaa":"<base64-encoded-governance-vaa>"}}' \
  --from <any-key>
```

The contract validates the VAA signatures against the current guardian set, then:
1. Stores the new guardian set at `index + 1`
2. Sets the old guardian set to expire after `guardian_set_expirity` seconds (86400 = 24h)

### Fallback: Migration via Governance

Since the contract admin is the governance module, a contract migration can force-update
guardian sets as a last resort (e.g., if the current guardian set is compromised):

```json
{
  "messages": [
    {
      "@type": "/cosmwasm.wasm.v1.MsgMigrateContract",
      "sender": "akash10d07y265gmmuvt4z0w9aw880jnsr700jhe7z0f",
      "contract": "<wormhole-contract-address>",
      "code_id": "<new-code-id>",
      "msg": "{}"
    }
  ],
  "deposit": "100000000uakt",
  "title": "Migrate Wormhole Contract",
  "summary": "Emergency guardian set update via contract migration"
}
```

This requires first storing a new contract binary (also via governance proposal with
`MsgStoreCode`), then migrating to it.

## Reference

- Pyth `UpdateConfig`: `contracts/pyth/src/msg.rs` — `ExecuteMsg::UpdateConfig`
- Wormhole `SubmitVAA`: `contracts/wormhole/src/msg.rs` — `ExecuteMsg::SubmitVAA`
- Guardian set update logic: `contracts/wormhole/src/contract.rs` — `vaa_update_guardian_set()`
- Contract instantiation (admin = gov): `upgrades/software/v2.0.0/wasm.go`
