# Sink settlement reference

The current service contract is [canonical Sink](sink-migration-v2.md). IDs contain owner, height and sequence. SDK types are `Sink_ID`, `Sink_Status`, `RecordData`, `ReporterPolicy`, `SolanaEvidence` and `TransferState` from the generated Akash v1 barrel.

`Transfer` accepts one composite ID. `Transfers` supports owner and financial-status filters with native pagination. Unfiltered global history is chronological. `Disputes` includes every held terminal record. `Account` exposes normalized account amount usage. `State` includes params, five financial amount/count pairs, current reporter policy, normalized send/refund usage and the canonical refund queue head.

Every page in a scan uses one committed height. Missing or pruned state is an error. There is no scalar lookup, synthetic public status, OpenTransfers endpoint or 1-to-last-ID scan.

A report uses `MsgReportOutcome` with current policy version, exact round and flat observation slot and signature. Quorum freezes the certificate. The first opposite observation creates a conflict; subsequent reports have no effect. `MsgVetoVoid` can stop an unpaid refund during its challenge. Governance resolves stale or conflicting authorization with `MsgRestartTransfer`, `MsgConfirmMint` or `MsgConfirmVoid`, always carrying `expected_round`.

Only `MsgFinalizeRefund` pays. It needs the complete recovery contract in the recovery guide. A void order, maturity or an absent Solana status cannot replace finalized void evidence.

`TransferState.status` contains the financial status. `SettlementData.policy` embeds the authorizing policy. `governance_observation`, flat veto fields and permanent `void_authorization` survive the applicable lifecycle transitions. `challenge_end` is integer Unix seconds. Status 3 maps to HTTP `void_requested`; it does not describe a confirmed Solana void or promise a refund.
