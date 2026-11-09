# Mainnet contract rehearsal: verified locally

On 2026-11-09, the complete contract ceremony passed on a fresh Solana 2.3.13
local validator using the published mainnet binaries and program IDs. The real
Squads v4 program controlled a disposable **2-of-5 multisig with a zero-second
time lock**. No mainnet transactions were sent.

Build source: `431953bb96a3cfdd1e1888210cae6dbf7d05a4ad`.
[Published artifacts](../deployments/mainnet/README.md) remain unchanged.
The rehearsal and documentation changes do not require a contract rebuild.

## What passed

1. Deploy all three programs with binary-sized allocations; compare their
   on-chain bytes against the published hashes.
2. Initialize the access manager and mint with zero supply, six decimals,
   no freeze authority, and minting disabled. Enable explicit open relaying.
3. Set the approved metadata URI and Squads vault as pause guardian.
4. Initialize the light client and bridge atomically, using the launch mint
   caps: 1,000,000 AKT per transfer and 10,000,000 AKT per day.
5. Transfer all three upgrade authorities and sole administration to Squads
   atomically. Verify that the retired ceremony signer is rejected.
6. Relay a signed test header as an unlisted signer, advancing the client from
   height 1 to 3. Reject that signer's attempt to prune consensus state.
7. Activate through Squads. Reject execution with one approval; execute with
   two approvals and no configured time-lock delay.
8. Pause through Squads and reject a mint with `BridgeDisabled`. Confirm no
   transfer status or token supply was created by the rejected transaction.
9. Resume through Squads. Reject an altered amount, then verify a **516-byte
   real Akash app proof** and mint exactly **123.456789 test AKT** to the proven
   recipient. The mint used **129,384 compute units**.
10. Reject a duplicate mint. Verify the recipient balance, total supply and
    permanent transfer status, then pause again through Squads.

The final client was active at height 3, relaying remained open, all program
upgrade authorities and administration belonged to the local Squads vault,
and minting was paused. Initial supply was zero; the later test mint came from
an ordinary signed deposit, with no treasury allocation or special seed path.

Deployment-tool tests (7), packaging-tool tests (2), operation tests (59),
TypeScript checking and scoped shell/format checks also passed. Packaging tests
did not build or publish service binaries.

## Reproduce

Use the prepared workspace, Node 24 dependencies, Solana CLI 2.3.13, the
repository's Go test environment and local test header encoder. Set
`PROGRAM_KEYS` to the existing program-key directory outside the repository.
The funded mainnet ceremony key is not used. Setup prerequisites are described
in the [ceremony guide](mainnet-ceremony.md#1-prepare-the-reviewed-release).

```bash
cd contracts/solana
npm run deploy:programs -- check --manifest "$PWD/deployments/mainnet/manifest.json"
AKT_REHEARSAL_TIME_LOCK=0 npm run rehearse:ceremony --   "$PWD/deployments/mainnet/manifest.json" "$PROGRAM_KEYS"
```

The run prints its evidence directory under the node repository's
`.cache/solana/run/`. The accepted run was `ceremony-rehearsal.tJMDkM`.
It records program verification, decoded/executed transactions, open-relay
results, rejection checks, the mint receipt and final state. Disposable keys,
the ledger and private deployment logs are removed automatically.

Two earlier attempts exposed errors in the newly added test harness: a copied
header object was not assigned back into its template, and intermediate header
reads used a different commitment from the existing relayer. Both were corrected;
the accepted run started again from an empty ledger. Production program code,
transport code, finality requirements and compiled artifacts were unchanged.

## Scope and mainnet prerequisites

The proof comes from a signed `MsgSink` committed by the real Akash app in an
isolated test. Its root is carried in a header signed by synthetic local
validators, starting from an explicitly trusted local checkpoint. This verifies
the contract sequence, signatures and proof execution together; it does not
authenticate a mainnet checkpoint or reproduce public-network conditions.

The separate Linux service deployment, reporter settlement round-trip, public
portal UI and control by the actual mainnet Squads members are outside this run.
Before mainnet writes, complete the independent build comparison on another
machine and human release review, including the existing
[compiler diagnostics](../deployments/mainnet/README.md#verification).
Before checkpoint setup, agree the real checkpoint, trusting period and clock
drift, and have the relayer payer and service ready. Confirm actual Squads member
control before handover. The launch time lock is confirmed as **0 seconds**.

Use the [command-by-command mainnet plan](mainnet-launch-commands.md) for the
live ceremony. Run one block at a time and review the result before continuing.
