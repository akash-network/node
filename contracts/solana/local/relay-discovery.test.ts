import assert from "node:assert/strict";
import { rm } from "node:fs/promises";
import { join } from "node:path";
import test from "node:test";
import { temporaryDirectory } from "./test-workspace";
import {
  EpochUsage,
  QueryStateResponse,
  ReporterPolicy,
  Sink_Params as Params,
  Sink_Status as Status,
  Totals,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { AnchorProvider, Wallet, web3 } from "@coral-xyz/anchor";
import { parseSinkId } from "../shared/id";
import { testTransfer } from "../tests/sink-fixtures";
import { relayContext } from "./relay";
import {
  FakeSolana,
  testBridge,
  relayTestConfig,
  HEIGHT,
  NOW,
} from "./relay-test-fixture";

const minted = (sequence: bigint, height: bigint) =>
  TransferState.fromPartial({
    ...testTransfer(sequence, height),
    status: Status.STATUS_MINTED,
    settlement: {
      round: 1n,
      policy: { version: 1n },
    },
  });

function events(lines: string[], event: string) {
  return lines.flatMap((line) => {
    const value: unknown = JSON.parse(line);
    return typeof value === "object" &&
      value !== null &&
      "event" in value &&
      value.event === event
      ? [value]
      : [];
  });
}

test("a discovery failure still relays open transfers and retries history from the saved checkpoint", async (t) => {
  const stateDir = await temporaryDirectory("relay-discovery-");
  t.after(() => rm(stateDir, { recursive: true, force: true }));
  const connection = new FakeSolana();
  const wallet = new AnchorProvider(
    connection,
    new Wallet(web3.Keypair.generate()),
    { commitment: "confirmed" },
  );
  const context = await relayContext({
    config: relayTestConfig(stateDir),
    wallet,
  });
  t.after(() => context.close());
  await testBridge(connection, context);
  const open = testTransfer(1n, 50n),
    older = minted(1n, 10n),
    newer = minted(1n, 60n);
  const [openId, newerId] = [open, newer].map((transfer) =>
    parseSinkId(transfer.id),
  );
  let historyFails = true;
  const historyCursor = new Uint8Array([255, 3, 19]);
  const requested: (Uint8Array | undefined)[] = [];
  const page = (transfers: TransferState[], nextKey = new Uint8Array()) => ({
    transfers,
    nextKey,
  });
  context.akash.state = async () => ({
    ...QueryStateResponse.fromPartial({}),
    params: Params.fromPartial({}),
    policy: ReporterPolicy.fromPartial({}),
    sendUsage: EpochUsage.fromPartial({}),
    refundUsage: EpochUsage.fromPartial({}),
    totals: Totals.fromPartial({}),
    height: HEIGHT,
    time: BigInt(NOW),
  });
  context.akash.disputes = async (height) => {
    assert.equal(height, HEIGHT);
    return page([]);
  };
  context.akash.transfers = async (height, options = {}) => {
    assert.equal(height, HEIGHT);
    if (options.status === Status.STATUS_TRANSFER_PENDING) return page([open]);
    if (options.status !== undefined) return page([]);
    const key = options.key;
    requested.push(key);
    if (!key?.length) return page([older], historyCursor);
    assert.deepEqual(Array.from(key), Array.from(historyCursor));
    if (historyFails) throw new Error("history height pruned");
    return page([newer]);
  };
  const logs: string[] = [],
    alerts: string[] = [];
  t.mock.method(console, "log", (line: string) => logs.push(line));
  t.mock.method(console, "error", (line: string) => alerts.push(line));
  const waitingFor = (id: string) =>
    events(logs, "relay-wait").some(
      (value) => "id" in value && value.id === id,
    );

  await context.tick();
  assert.deepEqual(
    events(alerts, "relay-discovery-failed"),
    [{ event: "relay-discovery-failed", error: "history height pruned" }],
    "the failed history page is alerted",
  );
  assert.ok(waitingFor(openId), "the open transfer is still planned");
  assert.deepEqual(requested, [undefined, historyCursor]);

  historyFails = false;
  alerts.length = 0;
  await context.tick();
  assert.deepEqual(events(alerts, "relay-discovery-failed"), []);
  assert.deepEqual(
    requested.slice(2),
    [undefined, historyCursor],
    "history restarts from the saved checkpoint, not the failed page",
  );
  assert.ok(waitingFor(newerId), "the discovered transfer is planned");

  await context.tick();
  assert.deepEqual(
    requested.slice(4).map((key) => Array.from(key ?? [])),
    [Array.from(historyCursor)],
    "a complete discovery saves its checkpoint",
  );
});
