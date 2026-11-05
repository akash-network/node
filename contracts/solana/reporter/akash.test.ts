import { testTransfer, testId, testSnapshot } from "../tests/sink-fixtures";
import { sinkIdParts } from "../shared/id";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import test from "node:test";
import { BaseAccount } from "cosmjs-types/cosmos/auth/v1beta1/auth";
import { QueryAccountResponse } from "cosmjs-types/cosmos/auth/v1beta1/query";
import { PubKey } from "cosmjs-types/cosmos/crypto/secp256k1/keys";
import { AkashClient, SinkRecordNotFoundError } from "./akash";
import {
  QueryTransfersRequest,
  QueryTransfersResponse,
  QueryStateResponse,
  Sink_QueryParamsResponse as QueryParamsResponse,
  QueryReportersRequest,
  QueryReportersResponse,
  QueryTransferRequest,
  QueryTransferResponse,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";

test("queries sink state and transfer at one committed height; rejects stale/wrong-chain RPC", async (t) => {
  const record = testTransfer();
  const snapshotState = testSnapshot();
  const state = QueryStateResponse.fromPartial({
    ...snapshotState,
    sendUsage: snapshotState.refundUsage,
    refundHead: sinkIdParts(testId()),
  });
  const transfer = QueryTransferResponse.fromPartial({ transfer: record });
  const status = {
    node_info: {
      id: "01".repeat(20),
      listen_addr: "tcp://127.0.0.1:26656",
      network: "akash-1",
      version: "0.38.21",
      channels: "",
      moniker: "reporter test",
      other: {},
      protocol_version: { app: "0", block: "11", p2p: "8" },
    },
    sync_info: {
      latest_block_hash: "02".repeat(32),
      latest_app_hash: "03".repeat(32),
      latest_block_height: "43", // Advertised before the app commits it.
      latest_block_time: new Date().toISOString(),
      catching_up: false,
    },
    validator_info: {
      address: "04".repeat(20),
      pub_key: {
        type: "tendermint/PubKeyEd25519",
        value: Buffer.alloc(32, 5).toString("base64"),
      },
      voting_power: "1",
    },
  };
  const requests: string[] = [];
  const openRequests: [Uint8Array | undefined, bigint | undefined][] = [];
  let openReply: object = {
    code: 0,
    value: Buffer.from(
      QueryTransfersResponse.encode(
        QueryTransfersResponse.fromPartial({
          transfers: [record],
          pagination: { nextKey: new Uint8Array([1, 2]) },
        }),
      ).finish(),
    ).toString("base64"),
  };
  let queryHeight = "42";
  let mismatchedPath = "";
  let transferFailure: { code: number; codespace: string } | undefined;
  const key = Buffer.alloc(33, 2);
  const baseAccount = (pubKey?: Uint8Array) => ({
    typeUrl: "/cosmos.auth.v1beta1.BaseAccount",
    value: BaseAccount.encode(
      BaseAccount.fromPartial({
        address: "akash1test",
        accountNumber: 7n,
        sequence: 3n,
        pubKey: pubKey && {
          typeUrl: "/cosmos.crypto.secp256k1.PubKey",
          value: PubKey.encode({ key: pubKey }).finish(),
        },
      }),
    ).finish(),
  });
  let account = baseAccount(key);
  const server = createServer(async (req, res) => {
    try {
      const chunks: Buffer[] = [];
      for await (const chunk of req) chunks.push(Buffer.from(chunk));
      const body: unknown = JSON.parse(Buffer.concat(chunks).toString());
      assert.ok(
        typeof body === "object" &&
          body !== null &&
          "method" in body &&
          "id" in body,
      );
      let result: unknown;
      if (body.method === "status") result = status;
      else if (body.method === "abci_info")
        result = {
          response: {
            data: "akash",
            last_block_height: "42",
            last_block_app_hash: "03".repeat(32),
          },
        };
      else if (body.method === "block") {
        assert.ok(
          "params" in body &&
            typeof body.params === "object" &&
            body.params !== null &&
            "height" in body.params,
        );
        assert.equal(body.params.height, "42");
        result = {
          block_id: {
            hash: "03".repeat(32),
            parts: { total: 1, hash: "04".repeat(32) },
          },
          block: {
            header: {
              version: { block: "11", app: "0" },
              chain_id: "akash-1",
              height: "42",
              time: status.sync_info.latest_block_time,
              last_block_id: { hash: "" },
              last_commit_hash: "",
              data_hash: "",
              validators_hash: "",
              next_validators_hash: "",
              consensus_hash: "",
              app_hash: "",
              last_results_hash: "",
              evidence_hash: "",
              proposer_address: "05".repeat(20),
            },
            data: { txs: [] },
            last_commit: { block_id: { hash: "" } },
          },
        };
      } else {
        assert.equal(body.method, "abci_query");
        assert.ok(
          "params" in body &&
            typeof body.params === "object" &&
            body.params !== null,
        );
        const params = body.params;
        assert.ok(
          "path" in params &&
            "height" in params &&
            "data" in params &&
            typeof params.data === "string",
        );
        assert.equal(params.height, "42");
        assert.ok(typeof params.path === "string");
        requests.push(params.path);
        let value: Uint8Array;
        if (params.path === "/akash.sink.v1.Query/Transfers") {
          const request = QueryTransfersRequest.decode(
            Buffer.from(params.data.replace(/^0x/, ""), "hex"),
          );
          openRequests.push([
            request.pagination?.key,
            request.pagination?.limit,
          ]);
          result = { response: { height: queryHeight, ...openReply } };
          res.setHeader("content-type", "application/json");
          res.end(JSON.stringify({ jsonrpc: "2.0", id: body.id, result }));
          return;
        }
        if (params.path === "/akash.sink.v1.Query/State")
          value = QueryStateResponse.encode(state).finish();
        else if (params.path === "/akash.sink.v1.Query/Params")
          value = QueryParamsResponse.encode({
            params: snapshotState.params,
          }).finish();
        else if (params.path === "/akash.sink.v1.Query/Reporters") {
          assert.equal(
            QueryReportersRequest.decode(
              Buffer.from(params.data.replace(/^0x/, ""), "hex"),
            ).version,
            0n,
          );
          value = QueryReportersResponse.encode({
            reporters: [snapshotState.policy],
          }).finish();
        } else if (params.path === "/cosmos.auth.v1beta1.Query/Account")
          value = QueryAccountResponse.encode({ account }).finish();
        else {
          assert.equal(params.path, "/akash.sink.v1.Query/Transfer");
          assert.deepEqual(
            QueryTransferRequest.decode(
              Buffer.from(params.data.replace(/^0x/, ""), "hex"),
            ).id,
            sinkIdParts(testId()),
          );
          if (transferFailure) {
            res.setHeader("content-type", "application/json");
            res.end(
              JSON.stringify({
                jsonrpc: "2.0",
                id: body.id,
                result: {
                  response: { ...transferFailure, height: queryHeight },
                },
              }),
            );
            return;
          }
          value = QueryTransferResponse.encode(transfer).finish();
        }
        result = {
          response: {
            code: 0,
            height: params.path === mismatchedPath ? "41" : queryHeight,
            value: Buffer.from(value).toString("base64"),
          },
        };
      }
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({ jsonrpc: "2.0", id: body.id, result }));
    } catch (error) {
      res.statusCode = 500;
      res.end(error instanceof Error ? error.message : "bad test request");
    }
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(
    () =>
      new Promise<void>((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      ),
  );
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  const client = new AkashClient(
    `http://127.0.0.1:${address.port}`,
    "akash-1",
    60,
  );
  t.after(() => client.close());
  const snapshot = await client.snapshot(testId());
  assert.equal(snapshot.policy.version, 1n);
  assert.equal(snapshot.epoch, 57n);
  assert.deepEqual(snapshot.transfer, record);
  assert.deepEqual(requests, [
    "/akash.sink.v1.Query/State",
    "/akash.sink.v1.Query/Params",
    "/akash.sink.v1.Query/Reporters",
    "/akash.sink.v1.Query/Transfer",
  ]);
  const page = await client.transfers(42, {
    key: new Uint8Array([9]),
    limit: 500,
  });
  assert.deepEqual(
    openRequests.map(([key, limit]) => [Array.from(key ?? []), limit]),
    [[[9], 500n]],
  );
  assert.deepEqual(Array.from(page.nextKey), [1, 2]);
  assert.deepEqual(page.transfers[0], record);
  for (const code of [6, 18, 22]) {
    openReply = { code, codespace: "sdk", log: "state unavailable or pruned" };
    await assert.rejects(() => client.transfers(42));
  }
  // The account key comes from the same committed height.
  assert.deepEqual(
    Buffer.from((await client.accountPublicKey("akash1test")) ?? []),
    key,
  );
  await assert.rejects(
    () => client.accountPublicKey("akash1other"),
    /account address mismatch/,
  );
  account = baseAccount();
  assert.equal(await client.accountPublicKey("akash1test"), undefined);
  assert.equal(requests.at(-1), "/cosmos.auth.v1beta1.Query/Account");
  transferFailure = { code: 22, codespace: "sdk" };
  await assert.rejects(
    () => client.transfer(testId(), 42),
    SinkRecordNotFoundError,
  );
  for (const code of [6, 18, 26, 38]) {
    transferFailure = { code, codespace: "sdk" };
    await assert.rejects(
      () => client.transfer(testId(), 42),
      (error) =>
        error instanceof Error && !(error instanceof SinkRecordNotFoundError),
    );
  }
  transferFailure = undefined;
  assert.ok(state.sendUsage && state.refundUsage);
  state.refundUsage.epoch--;
  await assert.rejects(() => client.state(), /quota usage epoch mismatch/);
  state.refundUsage.epoch++;
  state.epoch = 0n;
  state.sendUsage.epoch = 0n;
  state.refundUsage.epoch = 0n;
  assert.equal((await client.state()).epoch, 0n);
  state.refundUsage.amount = { denom: "uakt", amount: "1" };
  await assert.rejects(() => client.state(), /usage before quota epoch starts/);
  state.refundUsage.amount.amount = "0";
  for (const path of ["State", "Params", "Reporters"]) {
    mismatchedPath = `/akash.sink.v1.Query/${path}`;
    await assert.rejects(() => client.state(), /height mismatch/);
  }
  mismatchedPath = "";
  queryHeight = "41";
  await assert.rejects(() => client.state(), /height mismatch/);
  status.node_info.network = "other-chain";
  await assert.rejects(() => client.state(), /wrong Akash chain/);
  status.node_info.network = "akash-1";
  status.sync_info.catching_up = true;
  await assert.rejects(() => client.state(), /syncing/);
  status.sync_info.catching_up = false;
  status.sync_info.latest_block_time = new Date(0).toISOString();
  await assert.rejects(() => client.state(), /stale Akash node/);
});
