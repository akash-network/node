import {
  Hold,
  MsgSink,
  MsgSinkResponse,
  Sink_Params,
  Sink_Status as Status,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import type { comet38 } from "@cosmjs/tendermint-rpc";
import { TxMsgData } from "cosmjs-types/cosmos/base/abci/v1beta1/abci";
import { TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import type { AddressInfo } from "node:net";
import test from "node:test";
import { AkashClient } from "../reporter/akash";
import { sinkIdParts } from "../shared/id";
import {
  SINK_MSG_TYPE,
  SINK_RESPONSE_TYPE,
  sinkMessage,
  sinkResponseId,
} from "../shared/sink";
import { testId, testSnapshot, testTransfer } from "../tests/sink-fixtures";
import {
  assertOrigin,
  extensionOrigins,
  logFailure,
  proxyRequest,
  restPath,
  rpcRequest,
} from "./team-proxy";
import {
  akashSettlement,
  ApiFailure,
  assertTransactionHash,
  bindDepositRecord,
  bridgeLimits,
  depositFromRecord,
  depositFromTransaction,
  lightClientStatus,
  maxEpochAccountAmountUakt,
  minTransferUakt,
  networkIdentity,
  ownerHistory,
  receiptLookup,
  transactionHash,
  walletHistoryAddress,
} from "./team-receipt";

const sender = toBech32("akash", new Uint8Array(20).fill(1));
const sink = toBech32("akash", new Uint8Array(20).fill(2));
const recipient = web3.Keypair.generate().publicKey;
test("network keys include both genesis identities, program, mint and sink", () => {
  const identity = {
    chainId: "akash-1",
    akashGenesisTime: "2026-10-12T15:29:30.455284000Z",
    solanaGenesisHash: "genesis",
    program: "program",
    mint: "mint",
    sinkAddress: sink,
  };
  const network = networkIdentity(identity);
  assert.equal(
    network.networkKey,
    JSON.stringify([
      "akash-1",
      "2026-10-12T15:29:30.455284000Z",
      "genesis",
      "program",
      "mint",
      sink,
    ]),
  );
  for (const rebuilt of [
    { akashGenesisTime: "2026-10-18T09:00:00.000000000Z" },
    { solanaGenesisHash: "rebuilt" },
  ])
    assert.notEqual(
      networkIdentity({ ...identity, ...rebuilt }).networkKey,
      network.networkKey,
    );
  for (const value of [
    null,
    "",
    "a".repeat(63),
    "g".repeat(64),
    "a".repeat(64) + "'",
  ])
    assert.throws(() => transactionHash(value));
});

test("RPC rejects batches, admin methods, arbitrary tx searches and malformed writes", () => {
  const request = { jsonrpc: "2.0", id: 1, method: "status", params: {} };
  assert.equal(rpcRequest(request).method, "status");
  assert.equal(
    rpcRequest({
      ...request,
      method: "tx_search",
      params: { query: `tx.hash='${"A".repeat(64)}'` },
    }).method,
    "tx_search",
  );
  for (const input of [
    [request],
    { ...request, method: "unsafe_flush_mempool" },
    { ...request, method: "tx_search", params: { query: "tx.height>0" } },
    { ...request, method: "broadcast_tx_sync", params: { tx: "<script>" } },
    {
      ...request,
      method: "broadcast_tx_sync",
      params: { tx: "AQ==", other: true },
    },
  ])
    assert.throws(() => rpcRequest(input));
});

test("REST is fixed to required public query paths and bounded pagination", () => {
  const url = (path: string) => new URL(path, "https://test.example");
  assert.equal(
    restPath(url(`/rest/akash/cosmos/auth/v1beta1/accounts/${sender}`)),
    `/cosmos/auth/v1beta1/accounts/${sender}`,
  );
  assert.equal(
    restPath(url("/rest/akash/cosmos/base/tendermint/v1beta1/node_info")),
    "/cosmos/base/tendermint/v1beta1/node_info",
  );
  assert.ok(
    restPath(
      url(
        `/rest/akash/cosmos/bank/v1beta1/balances/${sender}?pagination.limit=1000`,
      ),
    ).endsWith("pagination.limit=1000"),
  );
  for (const path of [
    "/rest/akash/../../.git/config",
    "/rest/akash/http://evil.test",
    "/rest/akash/cosmos/tx/v1beta1/txs",
    `/rest/akash/cosmos/bank/v1beta1/balances/${sender}?pagination.limit=1001`,
    `/rest/akash/cosmos/bank/v1beta1/balances/${sender}?denom=uakt&denom=uakt`,
  ])
    assert.throws(() => restPath(url(path)));
});

test("REST accepts v2 identities and bounded native pagination, rejects scalar routes and overrides", () => {
  const url = (path: string) =>
    new URL(`/rest/akash${path}`, "https://test.example");
  const id = testId(7n, 42n, sender);
  for (const path of [
    "/akash/sink/v1/state",
    `/akash/sink/v1/transfers/${id}`,
    `/akash/sink/v1/accounts/${sender}`,
    `/akash/sink/v1/transfers?owner=${sender}&pagination.limit=500`,
    "/akash/sink/v1/disputes?pagination.limit=100",
  ])
    assert.equal(restPath(url(path)), path);
  for (const path of [
    "/akash/sink/v1/records/1",
    "/akash/sink/v1/transfers/1",
    `/akash/sink/v1/transfers/${id}?id.height=43`,
    "/akash/sink/v1/state?pagination.limit=1",
    "/akash/sink/v1/transfers?pagination.offset=1",
    "/akash/sink/v1/transfers?pagination.limit=501",
  ])
    assert.throws(() => restPath(url(path)));
});

test("origin allows exact site and configured extensions, rejects other sites and opaque origins", () => {
  const allowed = new Set(["https://test.example", ...extensionOrigins()]);
  assertOrigin("https://test.example", allowed);
  assertOrigin("chrome-extension://dmkamcknogkgcdfhhbddcghachkejeap", allowed);
  assertOrigin(undefined, allowed);
  for (const origin of [
    "null",
    "https://test.example.evil.test",
    "http://test.example",
    "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  ])
    assert.throws(() => assertOrigin(origin, allowed));
  assert.throws(() => extensionOrigins("https://evil.test"));
});

test("wallet history accepts only a canonical address, never a search expression", () => {
  assert.equal(walletHistoryAddress(new URLSearchParams({ sender })), sender);
  for (const query of [
    "",
    "sender=bad",
    `sender=${sender}&sender=${sender}`,
    `sender=${sender}&page=2`,
    new URLSearchParams({ sender: `${sender}' OR tx.height>0` }).toString(),
    new URLSearchParams({ sender: sender.toUpperCase() }).toString(),
    new URLSearchParams({
      sender: toBech32("cosmos", new Uint8Array(20)),
    }).toString(),
  ]) {
    assert.throws(
      () => walletHistoryAddress(new URLSearchParams(query)),
      (error: unknown) => error instanceof ApiFailure && error.status === 400,
    );
  }
});

test("unavailable upstreams log their cause; clients get a generic message", async (t) => {
  const lines: string[] = [];
  t.mock.method(console, "error", (line: string) => lines.push(line));
  const closed = createServer();
  await new Promise<void>((resolve) => closed.listen(0, "127.0.0.1", resolve));
  const { port } = closed.address() as AddressInfo;
  await new Promise((resolve) => closed.close(resolve));
  const response = {
    statusCode: 200,
    setHeader() {},
    end() {},
  } as unknown as ServerResponse;
  await assert.rejects(
    proxyRequest(
      { method: "GET", headers: {} } as IncomingMessage,
      response,
      new URL("http://portal.test/rpc/akash/status"),
      {
        akashRpc: `http://127.0.0.1:${port}`,
        akashRest: `http://127.0.0.1:${port}`,
        allowedOrigins: new Set(),
      },
    ),
    (error: unknown) =>
      error instanceof ApiFailure &&
      error.status === 503 &&
      error.message === "Akash endpoint is temporarily unavailable",
  );
  assert.equal(lines.length, 1);
  const logged = JSON.parse(lines[0]);
  assert.equal(logged.event, "portal-proxy-failed");
  assert.equal(logged.method, "GET");
  assert.equal(logged.upstream, "/status");
  assert.equal(logged.error, "TypeError");
  assert.ok(typeof logged.cause === "string");

  const secret = `[${"7,".repeat(63)}7`;
  let parseError: unknown;
  try {
    JSON.parse(secret);
  } catch (error) {
    parseError = error;
  }
  logFailure("portal-request-failed", parseError, {
    method: "GET",
    path: "/api/config",
    status: 503,
  });
  logFailure("portal-request-failed", new Error("outer", { cause: "x" }));
  assert.deepEqual(JSON.parse(lines[1]), {
    event: "portal-request-failed",
    method: "GET",
    path: "/api/config",
    status: 503,
    error: "SyntaxError",
  });
  assert.ok(!lines[1].includes("7,7"));
  assert.deepEqual(JSON.parse(lines[2]), {
    event: "portal-request-failed",
    error: "Error",
    cause: "string",
  });
});

test("light client status matches the relayer's frozen and expiry rule", () => {
  const base = {
    frozenHeight: 0n,
    latestHeight: 500n,
    consensusTimestampNs: 1_000_000_123_000_000_000n,
    trustingPeriodSeconds: 86_400n,
    solanaTimeSeconds: 1_000_000_123n + 30n,
  };
  assert.deepEqual(lightClientStatus(base), {
    status: "active",
    latestHeight: "500",
    ageSeconds: 30,
    trustingPeriodSeconds: 86_400,
  });
  const deadline = 1_000_000_123n + 86_400n;
  assert.equal(
    lightClientStatus({ ...base, solanaTimeSeconds: deadline - 1n }).status,
    "active",
  );
  assert.equal(
    lightClientStatus({ ...base, solanaTimeSeconds: deadline }).status,
    "expired",
  );
  assert.equal(
    lightClientStatus({ ...base, solanaTimeSeconds: deadline }).ageSeconds,
    86_400,
  );
  assert.equal(
    lightClientStatus({ ...base, frozenHeight: 7n }).status,
    "frozen",
  );
  assert.equal(
    lightClientStatus({
      ...base,
      frozenHeight: 7n,
      solanaTimeSeconds: deadline,
    }).status,
    "frozen",
  );
});

function sinkFixture() {
  const record = testTransfer(7n, 42n, sender);
  record.data = {
    dest: recipient.toBase58(),
    amount: { denom: "uakt", amount: "1234567" },
  };
  const message = sinkMessage({
    sender,
    recipient: recipient.toBase58(),
    amountUakt: "1234567",
  });
  const bytes = TxRaw.encode(
    TxRaw.fromPartial({
      bodyBytes: TxBody.encode(
        TxBody.fromPartial({
          messages: [
            {
              typeUrl: SINK_MSG_TYPE,
              value: MsgSink.encode(message.value).finish(),
            },
          ],
        }),
      ).finish(),
      signatures: [new Uint8Array(64).fill(3)],
    }),
  ).finish();
  const responses = [
    {
      typeUrl: SINK_RESPONSE_TYPE,
      value: MsgSinkResponse.encode({ id: record.id }).finish(),
    },
  ];
  const tx: comet38.TxResponse = {
    tx: Uint8Array.from(bytes),
    hash: createHash("sha256").update(bytes).digest(),
    height: 42,
    index: 0,
    result: {
      code: 0,
      gasWanted: 500000n,
      gasUsed: 100000n,
      events: [],
      data: TxMsgData.encode({ data: [], msgResponses: responses }).finish(),
    },
  };
  return { tx, record, responses };
}
test("v2 receipt binds exact bytes, response owner/height/sequence and every immutable field without events", () => {
  const { tx, record, responses } = sinkFixture();
  const hash = Buffer.from(tx.hash).toString("hex").toUpperCase();
  assertTransactionHash(tx, hash);
  const deposit = depositFromTransaction(tx);
  assert.equal(deposit.id, testId(7n, 42n, sender));
  assert.deepEqual(depositFromRecord(record), deposit);
  bindDepositRecord(deposit, record);
  assert.equal(sinkResponseId(responses), deposit.id);
  assert.throws(() =>
    assertTransactionHash({ ...tx, tx: new Uint8Array([1]) }, hash),
  );
  for (const changed of [
    { ...deposit, id: testId(8n, 42n, sender) },
    { ...deposit, sender: sink },
    { ...deposit, height: "43" },
    { ...deposit, recipient: web3.Keypair.generate().publicKey.toBase58() },
    { ...deposit, amountUakt: "1234568" },
  ])
    assert.throws(() => bindDepositRecord(changed, record));
  for (const id of [
    sinkIdParts(testId(7n, 43n, sender)),
    sinkIdParts(testId(7n, 42n, sink)),
    undefined,
  ]) {
    const data = TxMsgData.encode({
      data: [],
      msgResponses: [
        {
          typeUrl: SINK_RESPONSE_TYPE,
          value: MsgSinkResponse.encode({ id }).finish(),
        },
      ],
    }).finish();
    assert.throws(
      () => depositFromTransaction({ ...tx, result: { ...tx.result, data } }),
      ApiFailure,
    );
  }
  assert.throws(
    () =>
      depositFromTransaction({
        ...tx,
        result: { ...tx.result, data: new Uint8Array() },
      }),
    ApiFailure,
  );
});
test("lookup rejects old scalar IDs and binds selectors exclusively", () => {
  const id = testId();
  assert.deepEqual(receiptLookup(new URLSearchParams({ id })), {
    kind: "transfer",
    id,
  });
  for (const params of [
    new URLSearchParams({ id: "1" }),
    new URLSearchParams({ id, tx: "a".repeat(64) }),
    new URLSearchParams({ id: `${sender}/01/1` }),
  ])
    assert.throws(() => receiptLookup(params));
});
test("a zero account cap is unlimited and missing Coins fail closed", () => {
  assert.throws(() => minTransferUakt(Sink_Params.fromPartial({})));
  const disabled = Sink_Params.fromPartial({
    enabled: false,
    minAmountPerTransfer: { denom: "uakt", amount: "0" },
  });
  assert.equal(minTransferUakt(disabled), "0");
  assert.throws(() => minTransferUakt({ ...disabled, enabled: true }));
  assert.equal(
    maxEpochAccountAmountUakt(
      Sink_Params.fromPartial({
        maxEpochAmountPerAccount: { denom: "uakt", amount: "0" },
      }),
    ),
    "0",
  );
  assert.equal(
    minTransferUakt(
      Sink_Params.fromPartial({
        minAmountPerTransfer: { denom: "uakt", amount: "1000000" },
      }),
    ),
    "1000000",
  );
  assert.equal(
    maxEpochAccountAmountUakt(
      Sink_Params.fromPartial({
        maxEpochAmountPerAccount: { denom: "uakt", amount: "9007199254740993" },
      }),
    ),
    "9007199254740993",
  );
});
test("owner history is fixed-height, hashless and independent of tx indexing; pruned state/block fails", async (t) => {
  const client = new AkashClient("http://127.0.0.1:9", "akash-1", 60);
  t.after(() => client.close());
  const { record } = sinkFixture(),
    heights: number[] = [];
  t.mock.method(
    client,
    "transfers",
    async (height: number, options: { owner: string }) => {
      heights.push(height);
      assert.equal(options.owner, sender);
      return { transfers: [record], nextKey: new Uint8Array() };
    },
  );
  t.mock.method(client.comet, "txSearch", () => {
    throw new Error("index disabled");
  });
  t.mock.method(client.comet, "status", async () => nodeStatus(1));
  t.mock.method(client.comet, "block", async () => ({
    block: {
      header: {
        chainId: "akash-1",
        height: 42,
        time: new Date("2026-11-02T12:00:00Z"),
      },
    },
  }));
  const network = networkIdentity({
    chainId: "akash-1",
    akashGenesisTime: "2026-11-02T00:00:00Z",
    solanaGenesisHash: "g",
    program: "p",
    mint: "m",
    sinkAddress: sink,
  });
  const result = await ownerHistory(client, network, sender, 50);
  assert.equal(result.version, 2);
  assert.equal(result.truncated, false);
  assert.deepEqual(heights, [50]);
  assert.equal(result.transfers[0].txHash, null);
  assert.equal(result.transfers[0].transferId, testId(7n, 42n, sender));
  assert.equal(result.transfers[0].createdAt, "2026-11-02T12:00:00.000Z");
  t.mock.method(client.comet, "block", async () => {
    throw new Error("pruned block");
  });
  await assert.rejects(
    () => ownerHistory(client, network, sender, 50),
    /pruned/,
  );
  t.mock.method(client, "transfers", async () => {
    throw new Error("pruned state");
  });
  await assert.rejects(
    () => ownerHistory(client, network, sender, 50),
    /pruned/,
  );
});

function nodeStatus(earliestBlockHeight?: number, network = "akash-1") {
  return { nodeInfo: { network }, syncInfo: { earliestBlockHeight } };
}
function historyNode(
  t: test.TestContext,
  heights: number[],
  options: { base?: number; reverse?: boolean } = {},
) {
  const client = new AkashClient("http://127.0.0.1:9", "akash-1", 60);
  t.after(() => client.close());
  const rows = heights
    .map((height) => testTransfer(1n, BigInt(height), sender))
    .sort((a, b) => Number(b.id?.height) - Number(a.id?.height));
  const requests: unknown[] = [],
    blocks: number[] = [];
  t.mock.method(
    client,
    "transfers",
    async (
      height: number,
      request: { owner?: string; limit?: number; reverse?: boolean },
    ) => {
      requests.push({ height, ...request });
      const limit = request.limit ?? 100,
        ordered = options.reverse === false ? [...rows].reverse() : rows;
      return {
        transfers: ordered.slice(0, limit),
        nextKey: new Uint8Array(ordered.length > limit ? [1] : []),
      };
    },
  );
  t.mock.method(client.comet, "status", async () =>
    nodeStatus(options.base ?? 1),
  );
  t.mock.method(client.comet, "block", async (height: number) => {
    blocks.push(height);
    return {
      block: {
        header: { chainId: "akash-1", height, time: new Date(height * 1000) },
      },
    };
  });
  return { client, requests, blocks };
}
const historyNetwork = networkIdentity({
  chainId: "akash-1",
  akashGenesisTime: "2026-11-02T00:00:00Z",
  solanaGenesisHash: "g",
  program: "p",
  mint: "m",
  sinkAddress: sink,
});
test("owner history is the newest 100 records at one height and flags older ones", async (t) => {
  const many = historyNode(
    t,
    Array.from({ length: 150 }, (_, i) => i + 1),
  );
  const found = await ownerHistory(many.client, historyNetwork, sender, 200);
  assert.deepEqual(many.requests, [
    { height: 200, owner: sender, limit: 100, reverse: true },
  ]);
  assert.equal(found.truncated, true);
  assert.deepEqual(
    found.transfers.map((transfer) => transfer.transferId),
    Array.from({ length: 100 }, (_, i) => testId(1n, BigInt(150 - i), sender)),
  );
  assert.equal(found.transfers[0].createdAt, new Date(150_000).toISOString());
  const exact = historyNode(
    t,
    Array.from({ length: 100 }, (_, i) => i + 1),
  );
  const all = await ownerHistory(exact.client, historyNetwork, sender, 200);
  assert.equal(all.transfers.length, 100);
  assert.equal(all.truncated, false);
  const ascending = historyNode(t, [3, 4], { reverse: false });
  await assert.rejects(
    () => ownerHistory(ascending.client, historyNetwork, sender, 200),
    /newest first/,
  );
});
test("imported or pruned deposits have no time; unavailable or mismatched blocks fail", async (t) => {
  const node = historyNode(t, [45, 39, 10], { base: 40 });
  const found = await ownerHistory(node.client, historyNetwork, sender, 50);
  assert.deepEqual(
    found.transfers.map((transfer) => transfer.createdAt),
    [new Date(45_000).toISOString(), null, null],
  );
  assert.deepEqual(node.blocks, [45], "no block read below the node's base");
  const failures: [string, (client: AkashClient) => void, RegExp][] = [
    [
      "block transport",
      (client) =>
        t.mock.method(client.comet, "block", async () => {
          throw new Error("block request timed out");
        }),
      /timed out/,
    ],
    [
      "block chain",
      (client) =>
        t.mock.method(client.comet, "block", async (height: number) => ({
          block: {
            header: { chainId: "other", height, time: new Date(0) },
          },
        })),
      /other/,
    ],
    [
      "block height",
      (client) =>
        t.mock.method(client.comet, "block", async () => ({
          block: {
            header: { chainId: "akash-1", height: 46, time: new Date(0) },
          },
        })),
      /46/,
    ],
    [
      "status transport",
      (client) =>
        t.mock.method(client.comet, "status", async () => {
          throw new Error("status request timed out");
        }),
      /timed out/,
    ],
    [
      "status chain",
      (client) =>
        t.mock.method(client.comet, "status", async () =>
          nodeStatus(40, "other"),
        ),
      /wrong Akash chain/,
    ],
    [
      "unknown base",
      (client) =>
        t.mock.method(client.comet, "status", async () => nodeStatus()),
      /block history unavailable/,
    ],
    [
      "base above snapshot",
      (client) =>
        t.mock.method(client.comet, "status", async () => nodeStatus(51)),
      /block history unavailable/,
    ],
  ];
  for (const [name, fault, expected] of failures) {
    const broken = historyNode(t, [45, 39, 10], { base: 40 });
    fault(broken.client);
    await assert.rejects(
      () => ownerHistory(broken.client, historyNetwork, sender, 50),
      expected,
      name,
    );
  }
});
test("v2 receipts expose four native statuses and terminal conflict holds", () => {
  const snapshot = testSnapshot(),
    state = { ...snapshot, height: 42 };
  const deadline = 100n;
  const cases = [
    [Status.STATUS_TRANSFER_PENDING, "transfer_pending"],
    [Status.STATUS_MINTED, "minted"],
    [Status.STATUS_VOIDED, "void_requested"],
    [Status.STATUS_REFUND_PENDING, "refund_pending"],
    [Status.STATUS_REFUNDED, "refunded"],
  ] as const;
  assert.ok(snapshot.transfer.settlement);
  for (const [status, expected] of cases) {
    const result = akashSettlement(
      {
        ...snapshot.transfer,
        status,
        settlement: {
          ...snapshot.transfer.settlement,
          hold: Hold.HOLD_CONFLICT,
          challengeEnd: deadline,
        },
      },
      state,
    );
    assert.equal(result.status, expected);
    assert.equal(result.hold, "conflict");
    assert.equal(
      result.challengeEndSeconds,
      status === Status.STATUS_REFUND_PENDING ? "100" : null,
    );
    assert.ok(!("sinkStatus" in result));
  }
  const params = Sink_Params.fromPartial({
    ...snapshot.params,
    enabled: true,
    minAmountPerTransfer: { denom: "uakt", amount: "1" },
    maxAmountPerTransfer: { denom: "uakt", amount: "100" },
    maxEpochAmountPerAccount: { denom: "uakt", amount: "1000" },
    maxTransferAmountPerEpoch: { denom: "uakt", amount: "10000" },
  });
  assert.ok(bridgeLimits(params, true).enabled);
  assert.equal(bridgeLimits(params, false).enabled, false);
  for (const field of [
    "minAmountPerTransfer",
    "maxAmountPerTransfer",
    "maxEpochAmountPerAccount",
    "maxTransferAmountPerEpoch",
  ] as const) {
    const invalid = [undefined, { denom: "other", amount: "1" }];
    if (field !== "maxEpochAmountPerAccount")
      invalid.push({ denom: "uakt", amount: "0" });
    for (const coin of invalid)
      assert.throws(() => bridgeLimits({ ...params, [field]: coin }, true));
  }
});
