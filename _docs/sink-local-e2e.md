# Test Sink locally with the CLI

Run Akash, Solana, a relayer and seven reporters with Docker Compose. Deposit
AKT, check the mint, then test a refund. No portal is needed.

## Contents

- [Set up the chains](sink-local-setup.md)
- [Test a mint](#test-a-mint)
- [Test a refund](#test-a-refund)
- [Stop and resume](#stop-and-resume)
- [Deploy an operated network](sink-deployment.md)

## Before you start

Complete [local setup](sink-local-setup.md), including the native upgrade and
reporter activation. Continue in the same Bash session so its variables and
`dc`, `tools`, `a` and `tx` functions are available.

Container preparation and Solana program readback are verified. The full local
upgrade-to-refund flow and paired restart remain unverified.

## Test a mint

Send 10 AKT to the fresh Solana wallet:

```bash
recipient=$(tools solana-keygen pubkey /workspace/.cache/sink-compose/recipient.json)
tx sink deposit "$recipient" 10000000uakt
# After inclusion, obtain owner/height/sequence from MsgSinkResponse or history.
sender=$(a keys show "$SINK_KEY" -a --keyring-backend test)
a query sink transfers --owner "$sender" -o json
```

Set `TRANSFER_ID` to the deposit's `owner/height/sequence`, then inspect it:

```bash
a query sink transfer "$TRANSFER_ID" -o json
a query sink state -o json
dc logs --tail 30 relayer reporter0
mint=$(jq -r .mint "$SINK_RUN/network.json")
tools node -e 'const {Connection,PublicKey}=require("@solana/web3.js");
  new Connection("http://127.0.0.1:8899","finalized").getTokenSupply(new PublicKey(process.argv[1])).then(r=>console.log(r.value))' "$mint"
```

Expect `STATUS_TRANSFER_PENDING`, then `STATUS_MINTED` after finalized Solana
execution and five matching reports. Compare before and after values: this
deposit adds 10,000,000 base units to SPL supply and Akash's minted backing.

## Test a refund

Create a token account that cannot receive the mint as a wallet, then send 3 AKT:

```bash
invalid=$(tools node --import tsx /workspace/tests/upgrade/sink/identities.cjs invalid-recipient)
tx sink deposit "$invalid" 3000000uakt
a query sink transfers --owner "$sender" -o json
```

Set `REFUND_ID` to this deposit's composite ID:

```bash
a query sink transfer "$REFUND_ID" -o json
a query bank balances "$sender" -o json
```

Expect a permanent Solana void and `STATUS_REFUND_PENDING` after five matching
reports. Keep the relayer running until the recorded challenge deadline. The
standard build requires at least 72 hours; [local setup](sink-local-setup.md#build-the-images)
explains the separate `sinkdev` option.

Reporters finalize eligible refunds automatically. To test the CLI explicitly:

```bash
tx sink finalize-refund "$REFUND_ID"
a query sink transfer "$REFUND_ID" -o json
a query sink state -o json
```

Before the deadline, finalization must fail. After payment, repeats must not
pay again. Expect `STATUS_REFUNDED`, no SPL mint for this deposit and exactly
3,000,000 uakt returned to the sender. Account for any sender-paid fee separately.
Holds and epoch caps can delay payment; do not override them to pass the test.

## Stop and resume

```bash
dc stop reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6 relayer
dc stop solana validator
# Restore the same variables and shell functions before resuming.
dc up -d --wait validator solana
dc up -d relayer reporter0 reporter1 reporter2 reporter3 reporter4 reporter5 reporter6
```

Keep the same ledgers and journals. Do not rerun `prepare` or `bootstrap`.
Compare chain identities, balances and both transfer records across restart.

Retain artifact hashes, transaction IDs and public query results. Do not share
`SINK_RUN` or raw genesis files; they contain keys. When finished, `dc down`
removes this project's containers and network. Delete only its exact `SINK_RUN`
when the test data is no longer needed.
