import assert from "node:assert/strict";
import Long from "long";
import {
  QueryOpenTransfersRequest,
  QueryOpenTransfersResponse,
  type TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { decodeRecord } from "../reporter/solana";

// x/sink OpenTransfers lists every non-terminal transfer (pending, voided or
// held) in ID order. next_start_after is the last returned ID while more
// remain, otherwise zero. Nodes without the query get the cursor scan.
export const OPEN_TRANSFERS_PATH = "/akash.sink.v1.Query/OpenTransfers";
export const OPEN_TRANSFERS_PAGE = 100;

export function encodeOpenTransfersRequest(startAfter: bigint, limit: number) {
  return QueryOpenTransfersRequest.encode(
    QueryOpenTransfersRequest.fromPartial({
      // Unsigned: a bare bigint would wrap above 2^63 - 1.
      startAfter: Long.fromString(startAfter.toString(), true),
      limit,
    }),
  ).finish();
}

export function decodeOpenTransfersResponse(bytes: Uint8Array): {
  transfers: TransferState[];
  nextStartAfter: bigint;
} {
  const response = QueryOpenTransfersResponse.decode(bytes);
  return {
    transfers: response.transfers,
    nextStartAfter: BigInt(response.nextStartAfter.toString()),
  };
}

type AbciReply = {
  code?: number;
  codespace?: string;
  height?: number;
  value: Uint8Array;
};
export interface AbciQuerier {
  abciQuery(params: {
    path: string;
    data: Uint8Array;
    height?: number;
    prove?: boolean;
  }): Promise<AbciReply>;
}

// BaseApp answers an unregistered gRPC path with sdk code 6, which is also
// how a gRPC Unimplemented status arrives.
export const unsupportedQuery = (reply: {
  code?: number;
  codespace?: string;
}) => reply.code === 6 && reply.codespace === "sdk";

// Every open transfer at one committed height, or undefined when the node
// predates the query.
export async function openTransfers(
  comet: AbciQuerier,
  height: number,
  limit = OPEN_TRANSFERS_PAGE,
): Promise<{ id: bigint; transfer: TransferState }[] | undefined> {
  const result: { id: bigint; transfer: TransferState }[] = [];
  let startAfter = 0n;
  for (let page = 0; ; page++) {
    assert.ok(page < 100_000, "OpenTransfers paging exceeded its bound");
    const reply = await comet.abciQuery({
      path: OPEN_TRANSFERS_PATH,
      data: encodeOpenTransfersRequest(startAfter, limit),
      height,
      prove: false,
    });
    if (page === 0 && unsupportedQuery(reply)) return undefined;
    assert.ok(
      !reply.code,
      `OpenTransfers failed (${reply.codespace}/${reply.code})`,
    );
    assert.equal(reply.height, height, "OpenTransfers height mismatch");
    const { transfers, nextStartAfter } = decodeOpenTransfersResponse(
      reply.value,
    );
    for (const transfer of transfers) {
      const id = decodeRecord(transfer.record).id;
      assert.ok(
        id > (result.at(-1)?.id ?? startAfter),
        "OpenTransfers out of ID order",
      );
      result.push({ id, transfer });
    }
    if (nextStartAfter === 0n) return result;
    assert.ok(
      nextStartAfter > startAfter && transfers.length > 0,
      "OpenTransfers cursor did not advance",
    );
    startAfter = nextStartAfter;
  }
}
