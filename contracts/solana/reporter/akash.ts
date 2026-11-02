import assert from "node:assert/strict";
import { type OfflineSigner } from "@cosmjs/proto-signing";
import { Comet38Client, HttpClient } from "@cosmjs/tendermint-rpc";
import Long from "long";
import {
  QueryClient,
  SigningStargateClient,
  accountFromAny,
} from "@cosmjs/stargate";
import {
  QueryAccountRequest,
  QueryAccountResponse,
} from "cosmjs-types/cosmos/auth/v1beta1/query";
import {
  MsgReportOutcome,
  MsgVetoVoid,
  MsgFinalizeRefund,
  QueryOpenTransfersRequest,
  QueryOpenTransfersResponse,
  QueryStateResponse,
  QueryTransferRequest,
  QueryTransferResponse,
  QuerySink,
  QuerySinkResponse,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { parseSinkId, sinkRegistry } from "../shared/sink";
import { decodeRecord } from "./solana";
import type { Message, Snapshot } from "./policy";

export class SinkRecordNotFoundError extends Error {}

export function reporterRegistry() {
  const registry = sinkRegistry();
  registry.register("/akash.sink.v1.MsgReportOutcome", MsgReportOutcome);
  registry.register("/akash.sink.v1.MsgVetoVoid", MsgVetoVoid);
  registry.register("/akash.sink.v1.MsgFinalizeRefund", MsgFinalizeRefund);
  return registry;
}

export class AkashClient {
  readonly comet: Comet38Client;
  private readonly query: QueryClient;
  constructor(
    url: string,
    private readonly chainId: string,
    private readonly maxAgeSeconds: number,
  ) {
    this.comet = Comet38Client.create(new HttpClient(url, 15_000));
    this.query = new QueryClient(this.comet);
  }
  close() {
    this.comet.disconnect();
  }

  async state() {
    const status = await this.comet.status();
    assert.equal(status.nodeInfo.network, this.chainId, "wrong Akash chain");
    assert.ok(!status.syncInfo.catchingUp, "Akash node is syncing");
    const age =
      Date.now() / 1000 -
      Math.floor(status.syncInfo.latestBlockTime.getTime() / 1000);
    assert.ok(age >= -10 && age <= this.maxAgeSeconds, "stale Akash node");
    // Comet may advertise H while BaseApp is still committing H. Query only a
    // height the application has committed, and use that block's clock.
    const info = await this.comet.abciInfo();
    const height = Math.min(
      status.syncInfo.latestBlockHeight,
      info.lastBlockHeight ?? 0,
    );
    assert.ok(
      height > 1 && Number.isSafeInteger(height),
      "Akash node has no queryable committed state",
    );
    const block = (await this.comet.block(height)).block.header;
    assert.equal(block.height, height, "Akash block height mismatch");
    assert.equal(block.chainId, this.chainId, "wrong Akash block chain");
    const time = Math.floor(block.time.getTime() / 1000);
    const committedAge = Date.now() / 1000 - time;
    assert.ok(
      committedAge >= -10 && committedAge <= this.maxAgeSeconds,
      "stale Akash committed state",
    );
    const reply = await this.query.queryAbci(
      "/akash.sink.v1.Query/State",
      new Uint8Array(),
      height,
    );
    assert.equal(reply.height, height, "Akash query height mismatch");
    const state = QueryStateResponse.decode(reply.value);
    assert.ok(state.params && state.totals, "missing sink state");
    return {
      params: state.params,
      totals: state.totals,
      height,
      time: BigInt(time),
    };
  }

  async transfer(id: bigint, height: number) {
    const reply = await this.query.queryAbci(
      "/akash.sink.v1.Query/Transfer",
      QueryTransferRequest.encode(
        QueryTransferRequest.fromPartial({ id }),
      ).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash transfer query height mismatch");
    const transfer = QueryTransferResponse.decode(reply.value).transfer;
    assert.ok(transfer?.settlement, "missing sink transfer");
    assert.equal(
      decodeRecord(transfer.record).id,
      id,
      "Akash transfer ID mismatch",
    );
    return transfer;
  }

  // One page of non-terminal transfers at `height`, in ascending ID order.
  // Undefined when the node predates the query; other errors throw.
  async openTransfers(startAfter: bigint, limit: number, height: number) {
    const reply = await this.comet.abciQuery({
      path: "/akash.sink.v1.Query/OpenTransfers",
      data: QueryOpenTransfersRequest.encode(
        QueryOpenTransfersRequest.fromPartial({
          startAfter: Long.fromString(startAfter.toString(), true),
          limit,
        }),
      ).finish(),
      height,
      prove: false,
    });
    if (
      reply.code === 6 &&
      reply.codespace === "sdk" &&
      reply.log?.startsWith("unknown query path")
    )
      return undefined;
    assert.ok(!reply.code, `OpenTransfers query failed (${reply.code})`);
    assert.equal(reply.height, height, "Akash open query height mismatch");
    const result = QueryOpenTransfersResponse.decode(reply.value);
    return {
      transfers: result.transfers,
      next: BigInt(result.nextStartAfter.toString()),
    };
  }

  async snapshot(id: bigint): Promise<Snapshot> {
    const state = await this.state();
    return { ...state, transfer: await this.transfer(id, state.height) };
  }

  async sink(id: string, height: number) {
    const canonicalId = parseSinkId(id);
    const reply = await this.comet.abciQuery({
      path: "/akash.sink.v1.Query/Sink",
      data: QuerySink.encode({ id: canonicalId }).finish(),
      height,
      prove: false,
    });
    // The pinned SDK maps gRPC NotFound to sdk/22 (ErrKeyNotFound).
    if (reply.code === 22 && reply.codespace === "sdk")
      throw new SinkRecordNotFoundError("Sink record not found");
    assert.ok(!reply.code, `Sink query failed (${reply.code})`);
    assert.equal(reply.height, height, "Akash sink query height mismatch");
    const result = QuerySinkResponse.decode(reply.value);
    assert.ok(result.transfer?.settlement, "missing sink transfer");
    assert.equal(
      decodeRecord(result.transfer.record).id.toString(),
      canonicalId,
      "Akash sink record ID mismatch",
    );
    return result;
  }

  // The account's secp256k1 key once it has signed a transaction, otherwise
  // undefined. Callers must still check that it derives the address.
  async accountPublicKey(address: string) {
    const { height } = await this.state();
    const reply = await this.query.queryAbci(
      "/cosmos.auth.v1beta1.Query/Account",
      QueryAccountRequest.encode({ address }).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash account query height mismatch");
    const { account } = QueryAccountResponse.decode(reply.value);
    assert.ok(account, "missing Akash account");
    const parsed = accountFromAny(account);
    assert.equal(parsed.address, address, "Akash account address mismatch");
    const { pubkey } = parsed;
    if (!pubkey) return undefined;
    assert.equal(
      pubkey.type,
      "tendermint/PubKeySecp256k1",
      "Akash account key is not secp256k1",
    );
    assert.ok(typeof pubkey.value === "string");
    return Uint8Array.from(Buffer.from(pubkey.value, "base64"));
  }

  broadcaster(
    signer: OfflineSigner,
    reporter: string,
    fee: { amount: string; gas: string },
  ) {
    const client = SigningStargateClient.createWithSigner(this.comet, signer, {
      registry: reporterRegistry(),
      broadcastTimeoutMs: 30_000,
      broadcastPollIntervalMs: 1000,
    });
    return async (message: Message) => {
      // Recheck identity and membership before signing. Epoch/round races and
      // refund eligibility are checked again by the native message handler.
      const state = await this.state();
      assert.ok(
        state.params.reporters.includes(reporter),
        "reporter was removed",
      );
      const tx = await client.signAndBroadcast(
        reporter,
        [message],
        {
          amount: [{ denom: "uakt", amount: fee.amount }],
          gas: fee.gas,
        },
        "AKT sink reporter",
        BigInt(state.height) + 20n,
      );
      assert.equal(
        tx.code,
        0,
        `sink transaction failed: ${tx.transactionHash} code ${tx.code}`,
      );
      return tx.transactionHash;
    };
  }
}
