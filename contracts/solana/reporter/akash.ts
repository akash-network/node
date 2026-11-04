import assert from "node:assert/strict";
import { type OfflineSigner } from "@cosmjs/proto-signing";
import { Comet38Client, HttpClient } from "@cosmjs/tendermint-rpc";
import {
  QueryClient,
  SigningStargateClient,
  accountFromAny,
} from "@cosmjs/stargate";
import {
  QueryAccountRequest as AuthAccountRequest,
  QueryAccountResponse as AuthAccountResponse,
} from "cosmjs-types/cosmos/auth/v1beta1/query";
import {
  MsgReportOutcome,
  MsgVetoVoid,
  MsgFinalizeRefund,
  QueryTransfersRequest,
  QueryTransfersResponse,
  QueryDisputesRequest,
  QueryDisputesResponse,
  QueryAccountRequest,
  QueryAccountResponse,
  Sink_Status as Status,
  Hold,
  QueryStateResponse,
  Sink_QueryParamsResponse as QueryParamsResponse,
  QueryReportersRequest,
  QueryReportersResponse,
  QueryTransferRequest,
  QueryTransferResponse,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { sinkRegistry } from "../shared/sink";
import { coinAmount, decodeRecord } from "../shared/proof";
import {
  parseSinkId,
  sinkIdParts,
  ownerBytes,
  type SinkId,
} from "../shared/id";
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
    const replies = await Promise.all([
      this.query.queryAbci(
        "/akash.sink.v1.Query/State",
        new Uint8Array(),
        height,
      ),
      this.query.queryAbci(
        "/akash.sink.v1.Query/Params",
        new Uint8Array(),
        height,
      ),
      this.query.queryAbci(
        "/akash.sink.v1.Query/Reporters",
        QueryReportersRequest.encode({ version: 0n }).finish(),
        height,
      ),
    ]);
    for (const reply of replies)
      assert.equal(reply.height, height, "Akash query height mismatch");
    const state = QueryStateResponse.decode(replies[0].value);
    const { params } = QueryParamsResponse.decode(replies[1].value);
    const { reporters } = QueryReportersResponse.decode(replies[2].value);
    assert.equal(reporters.length, 1, "missing current reporter policy");
    const policy = reporters[0];
    assert.ok(
      params &&
        state.totals &&
        policy.version > 0n &&
        state.sendUsage &&
        state.refundUsage,
      "missing sink state",
    );
    assert.ok(state.epoch <= 0x7fffffffffffffffn, "invalid quota epoch");
    for (const usage of [state.sendUsage, state.refundUsage]) {
      assert.equal(usage.epoch, state.epoch, "quota usage epoch mismatch");
      const amount = coinAmount(usage.amount);
      assert.ok(
        state.epoch > 0n || amount === 0n,
        "usage before quota epoch starts",
      );
    }
    return {
      ...state,
      params,
      policy,
      sendUsage: state.sendUsage,
      refundUsage: state.refundUsage,
      totals: state.totals,
      height,
      time: BigInt(time),
    };
  }

  async transfer(id: SinkId, height: number) {
    const reply = await this.comet.abciQuery({
      path: "/akash.sink.v1.Query/Transfer",
      data: QueryTransferRequest.encode({ id: sinkIdParts(id) }).finish(),
      height,
      prove: false,
    });
    // BaseApp maps the service's gRPC NotFound to sdk/22. Pruned state is
    // sdk/38 from CreateQueryContext and must remain an unavailable error.
    if (reply.code === 22 && reply.codespace === "sdk")
      throw new SinkRecordNotFoundError("Sink record not found");
    assert.ok(!reply.code, `Transfer query failed (${reply.code})`);
    assert.equal(reply.height, height, "Akash transfer query height mismatch");
    const transfer = QueryTransferResponse.decode(reply.value).transfer;
    assert.ok(transfer?.settlement, "missing sink transfer");
    assert.equal(decodeRecord(transfer).id, id, "Akash transfer ID mismatch");
    return transfer;
  }

  async transfers(
    height: number,
    options: {
      key?: Uint8Array;
      limit?: number;
      owner?: string;
      status?: Status;
      reverse?: boolean;
    } = {},
  ) {
    if (options.owner) ownerBytes(options.owner);
    const limit = options.limit ?? 100;
    assert.ok(
      Number.isSafeInteger(limit) && limit > 0 && limit <= 500,
      "invalid page limit",
    );
    const reply = await this.query.queryAbci(
      "/akash.sink.v1.Query/Transfers",
      QueryTransfersRequest.encode(
        QueryTransfersRequest.fromPartial({
          owner: options.owner,
          status: options.status,
          pagination: {
            key: options.key,
            limit: BigInt(limit),
            reverse: options.reverse,
          },
        }),
      ).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash transfers query height mismatch");
    const result = QueryTransfersResponse.decode(reply.value);
    assert.ok(
      result.pagination && result.transfers.length <= limit,
      "invalid transfers page",
    );
    for (const transfer of result.transfers) {
      const id = sinkIdParts(decodeRecord(transfer).id);
      assert.ok(transfer.settlement, "missing settlement");
      assert.ok(
        !options.owner || id.owner === options.owner,
        "owner filter mismatch",
      );
      assert.ok(
        options.status === undefined ||
          options.status === Status.STATUS_INVALID ||
          transfer.status === options.status,
        "status filter mismatch",
      );
    }
    return { transfers: result.transfers, nextKey: result.pagination.nextKey };
  }

  async disputes(
    height: number,
    key: Uint8Array = new Uint8Array(),
    limit = 100,
  ) {
    const reply = await this.query.queryAbci(
      "/akash.sink.v1.Query/Disputes",
      QueryDisputesRequest.encode(
        QueryDisputesRequest.fromPartial({
          pagination: { key, limit: BigInt(limit) },
        }),
      ).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash disputes query height mismatch");
    const result = QueryDisputesResponse.decode(reply.value);
    assert.ok(
      result.pagination && result.transfers.length <= limit,
      "invalid disputes page",
    );
    for (const transfer of result.transfers) {
      decodeRecord(transfer);
      assert.ok(
        transfer.settlement && transfer.settlement.hold !== Hold.HOLD_NONE,
        "unheld dispute",
      );
    }
    return { transfers: result.transfers, nextKey: result.pagination.nextKey };
  }

  async account(owner: string, height: number) {
    ownerBytes(owner);
    const reply = await this.query.queryAbci(
      "/akash.sink.v1.Query/Account",
      QueryAccountRequest.encode({ owner }).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash account query height mismatch");
    const result = QueryAccountResponse.decode(reply.value);
    assert.ok(result.usage, "missing account usage");
    return result;
  }

  async snapshot(id: SinkId): Promise<Snapshot> {
    const state = await this.state();
    return {
      ...state,
      refundHead: state.refundHead && parseSinkId(state.refundHead),
      transfer: await this.transfer(id, state.height),
    };
  }

  // The account's secp256k1 key once it has signed a transaction, otherwise
  // undefined. Callers must still check that it derives the address.
  async accountPublicKey(address: string) {
    const { height } = await this.state();
    const reply = await this.query.queryAbci(
      "/cosmos.auth.v1beta1.Query/Account",
      AuthAccountRequest.encode({ address }).finish(),
      height,
    );
    assert.equal(reply.height, height, "Akash account query height mismatch");
    const { account } = AuthAccountResponse.decode(reply.value);
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
        state.policy.members.includes(reporter),
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
