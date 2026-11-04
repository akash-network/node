import {
  EpochUsage,
  Sink_Params as Params,
  ReporterPolicy,
  Sink_Status as Status,
  Totals,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import type { Snapshot } from "../reporter/policy";
import { parseSinkId, sinkIdParts } from "../shared/id";
export const TEST_OWNER = toBech32("akash", new Uint8Array(20).fill(42));
export function testId(
  sequence: bigint | number = 1n,
  height = 1n,
  owner = TEST_OWNER,
) {
  return parseSinkId({ owner, height, sequence: BigInt(sequence) });
}
export function testTransfer(
  sequence: bigint | number = 1n,
  height = 1n,
  owner = TEST_OWNER,
) {
  return TransferState.fromPartial({
    id: sinkIdParts(testId(sequence, height, owner)),
    data: {
      dest: web3.Keypair.generate().publicKey.toBase58(),
      amount: { denom: "uakt", amount: "60" },
    },
    status: Status.STATUS_TRANSFER_PENDING,
    settlement: {
      round: 1n,
      policy: {
        version: 1n,
        members: ["reporter", "b", "c", "d"],
        threshold: "0.75",
      },
    },
  });
}
export function testSnapshot(): Snapshot {
  return {
    params: Params.fromPartial({
      reporters: ["reporter", "b", "c", "d"],
      reportersThreshold: "0.75",
      refundsEnabled: true,
      maxEpochRefund: { denom: "uakt", amount: "100" },
    }),
    totals: Totals.fromPartial({}),
    policy: ReporterPolicy.fromPartial({
      version: 1n,
      members: ["reporter", "b", "c", "d"],
      threshold: "0.75",
    }),
    refundUsage: EpochUsage.fromPartial({
      epoch: 57n,
      amount: { denom: "uakt", amount: "0" },
    }),
    time: 86410n,
    epoch: 57n,
    transfer: testTransfer(),
    refundHead: testId(),
  };
}
