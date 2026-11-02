import { Sink_Status as Status } from "@akashnetwork/chain-sdk/private-types/akash.v1";

// Reconciliation decisions without RPC access, so they can be tested directly.

// Nominal Solana slot time, used to age a status from its resolution slot.
export const SLOT_SECONDS = 0.4;

export function reconcileSettings(env: Record<string, string | undefined>) {
  const reportLagSeconds = Number(env.AKT_RECONCILE_REPORT_LAG_SECONDS ?? 600);
  if (!(Number.isSafeInteger(reportLagSeconds) && reportLagSeconds > 0))
    throw new Error(
      "AKT_RECONCILE_REPORT_LAG_SECONDS must be a positive integer",
    );
  return { reportLagSeconds };
}

// Paged status reads may return a later finalized slot than the page that
// pinned the snapshot. Statuses are permanent and record their resolution
// slot, so one resolved after the pinned slot did not exist in it.
export function inSnapshot(resolvedAtSlot: bigint, slot: number) {
  return resolvedAtSlot <= BigInt(slot);
}

export function transferAlerts(input: {
  id: bigint;
  akash: Status;
  solana?: "minted" | "voided";
  resolvedAtSlot?: bigint;
  slot: number;
  reportLagSeconds: number;
}) {
  const { id, akash, solana } = input;
  const alerts: string[] = [];
  if (
    solana === "minted" &&
    (akash === Status.STATUS_REFUNDED || akash === Status.STATUS_VOIDED)
  )
    alerts.push(`minted transfer ${id} has ${Status[akash]} on Akash`);
  if (akash === Status.STATUS_MINTED && solana !== "minted")
    alerts.push(`mint confirmation ${id} lacks finalized Solana mint`);
  if (akash === Status.STATUS_REFUNDED && solana !== "voided")
    alerts.push(`refund ${id} lacks finalized Solana void`);
  // Only a Solana void can authorize a refund. Without a status the transfer
  // can still mint, and the refund would then pay a second time.
  if (akash === Status.STATUS_VOIDED && solana === undefined)
    alerts.push(`void ${id} on Akash lacks any finalized Solana status`);
  if (
    solana !== undefined &&
    akash === Status.STATUS_PENDING &&
    input.resolvedAtSlot !== undefined
  ) {
    const lag = Math.max(0, input.slot - Number(input.resolvedAtSlot));
    const seconds = Math.round(lag * SLOT_SECONDS);
    if (seconds >= input.reportLagSeconds)
      alerts.push(
        `transfer ${id} ${solana} on Solana about ${seconds} s ago is still pending on Akash; reporters missed its evidence`,
      );
  }
  return alerts;
}
