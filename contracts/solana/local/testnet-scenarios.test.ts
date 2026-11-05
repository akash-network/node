import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";
import { runInNewContext } from "node:vm";
import test from "node:test";
import ts from "typescript";
import { QueryStateResponse } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { sinkIdParts } from "../shared/id";
import { coinAmount } from "../shared/proof";
import { testId } from "../tests/sink-fixtures";

const sourceText = readFile(
  new URL("./testnet-scenarios.ts", import.meta.url),
  "utf8",
);
const scenarioComplete = new Error("stopped after the one-validator scenario");
const restorationSteps = [
  "validator2",
  "validator3",
  "bridge",
  "relay.close",
  "node.disconnect",
  "sender.disconnect",
  "stack.up",
] as const;
type Restoration = (typeof restorationSteps)[number];

function deferred<T>() {
  let resolve: (value: T) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve: (value: T) => resolve(value) };
}

async function runScenarios({
  failures = new Map<string, unknown>(),
  upload = async (_target: number, tampered: boolean) => {
    if (tampered) throw new Error("UpdateClientFailed");
  },
} = {}) {
  const finished = deferred<unknown>();
  const validators = new Set([0, 1, 2, 3]);
  const restored: Restoration[] = [];
  const uploads: { target: number; tampered: boolean }[] = [];
  const commits: number[] = [];
  let height = 100;
  const process = { execPath: "/fake/node", exitCode: 0 };
  function fail(step: string) {
    if (failures.has(step)) throw failures.get(step);
  }
  function restore(step: Restoration) {
    restored.push(step);
    fail(step);
  }
  const execFile = Object.assign(
    () => assert.fail("expected promisified exec"),
    {
      [promisify.custom]: async () => {
        restore("stack.up");
        return { stdout: "services restored" };
      },
    },
  );
  const modules = new Map<string, unknown>([
    ["node:assert/strict", assert],
    ["node:child_process", { execFile }],
    ["node:crypto", {}],
    ["node:path", { join }],
    ["node:util", { promisify }],
    ["@akashnetwork/chain-sdk/private-types/akash.v1", {}],
    ["@coral-xyz/anchor", {}],
    ["@cosmjs/encoding", {}],
    ["@solana/spl-token", {}],
    ["cosmjs-types/cosmos/gov/v1/gov", {}],
    ["cosmjs-types/cosmos/gov/v1/tx", {}],
    ["../reporter/solana", {}],
    ["../shared/id", {}],
    ["../shared/proof", {}],
    ["../shared/sink", {}],
    ["./reconcile", {}],
    [
      "./common",
      {
        PROFILE: "testnetify",
        ROOT: "/fake/run",
        REPO: "/fake/solana",
        AKASH_RPC: "http://fake.invalid",
        json: async () => ({ sinkAddress: "sink" }),
        object: (value: unknown) => {
          assert.ok(value !== null && typeof value === "object");
          return value;
        },
        pda: (name: string) => name,
        programData: () => "program-data",
        sleep: async () => {},
        save: async () => {
          throw scenarioComplete;
        },
        cosmosSigner: async () => ({
          client: { disconnect: () => restore("sender.disconnect") },
        }),
      },
    ],
    [
      "./relay",
      {
        relayContext: async () => ({
          wallet: { publicKey: "wallet" },
          akash: { state: async () => ({ totals: { pendingCount: 0n } }) },
          program: {
            methods: {
              setBridgeEnabled: (enabled: boolean) => ({
                accountsStrict: () => ({
                  rpc: async () => {
                    assert.equal(enabled, true);
                    restore("bridge");
                  },
                }),
              }),
            },
          },
          update: async (target: number, tampered = false) => {
            uploads.push({ target, tampered });
            await upload(target, tampered);
          },
          close: () => restore("relay.close"),
        }),
      },
    ],
    [
      "./stack",
      {
        processes: async () => [{ name: "relayer" }, { name: "akash3" }],
        stop: async ({ name }: { name: string }) => {
          if (name === "akash3") validators.delete(3);
        },
      },
    ],
    [
      "./testnetify",
      {
        comet: () => ({
          validatorsAll: async () => ({ validators: [...validators] }),
          status: async () => ({ syncInfo: { latestBlockHeight: ++height } }),
          disconnect: () => restore("node.disconnect"),
        }),
        validatorName: (index: number) => `akash${index}`,
        startValidator: async (index: number) => {
          assert.ok(index === 2 || index === 3);
          restore(index === 2 ? "validator2" : "validator3");
          validators.add(index);
        },
      },
    ],
  ]);
  runInNewContext(
    ts.transpileModule(await sourceText, {
      compilerOptions: {
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.CommonJS,
        esModuleInterop: true,
      },
    }).outputText,
    {
      exports: {},
      require: (name: string) => {
        assert.ok(modules.has(name), `unexpected import ${name}`);
        return modules.get(name);
      },
      AggregateError,
      AbortSignal,
      process,
      console: { log: () => {}, error: finished.resolve },
      fetch: async (url: string) => {
        assert.equal(validators.has(3), false);
        fail("commit");
        commits.push(Number(new URL(url).searchParams.get("height")));
        return {
          json: async () => ({
            result: {
              signed_header: {
                commit: {
                  signatures: [2, 2, 2, 1].map((block_id_flag) => ({
                    block_id_flag,
                  })),
                },
              },
            },
          }),
        };
      },
    },
  );
  return {
    finished: finished.promise,
    validators,
    restored,
    uploads,
    commits,
    process,
    advance: (blocks: number) => {
      height += blocks;
    },
  };
}

test("validator 3 is online throughout slow uploads of its historical missing-signature commit", async () => {
  const started = [deferred<void>(), deferred<void>()];
  const release = [deferred<void>(), deferred<void>()];
  let count = 0;
  const run = await runScenarios({
    upload: async (_target, tampered) => {
      const index = count++;
      started[index].resolve();
      await release[index].promise;
      if (tampered) throw new Error("UpdateClientFailed");
    },
  });
  const online: boolean[] = [];
  for (let i = 0; i < 2; i++) {
    await Promise.race([
      started[i].promise,
      run.finished.then((error) => {
        throw error;
      }),
    ]);
    online.push(run.validators.has(3));
    run.advance(200);
    online.push(run.validators.has(3));
    release[i].resolve();
  }
  assert.equal(await run.finished, scenarioComplete);
  assert.deepEqual(online, [true, true, true, true]);
  assert.equal(run.commits.length, 1);
  assert.deepEqual(run.uploads, [
    { target: run.commits[0], tampered: true },
    { target: run.commits[0], tampered: false },
  ]);
});

test("scenario and stack restoration failures are both reported", async () => {
  const scenarioError = new Error("commit RPC failed");
  const cleanupError = new Error("expected four active validators");
  const run = await runScenarios({
    failures: new Map([
      ["commit", scenarioError],
      ["stack.up", cleanupError],
    ]),
  });
  const error = await run.finished;
  assert.ok(error instanceof AggregateError);
  assert.deepEqual(error.errors, [scenarioError, cleanupError]);
  assert.deepEqual(run.restored, restorationSteps);
  assert.equal(run.validators.has(3), true);
  assert.equal(run.process.exitCode, 1);
});

test("every restoration and client close is attempted after earlier cleanup failures", async () => {
  const steps = ["commit", ...restorationSteps];
  const failures = new Map(steps.map((step) => [step, new Error(step)]));
  const run = await runScenarios({ failures });
  const error = await run.finished;
  assert.ok(error instanceof AggregateError);
  assert.deepEqual(error.errors, [...failures.values()]);
  assert.deepEqual(run.restored, restorationSteps);
});

test("a lone scenario failure is rethrown unchanged, including falsy thrown values", async () => {
  for (const error of [new Error("commit failed"), undefined, null, 0]) {
    const run = await runScenarios({ failures: new Map([["commit", error]]) });
    assert.equal(await run.finished, error);
    assert.deepEqual(run.restored, restorationSteps);
    assert.equal(run.validators.has(3), true);
  }
});

test("an accepted tampered header still fails the scenario", async () => {
  const run = await runScenarios({ upload: async () => {} });
  const error = await run.finished;
  assert.ok(error instanceof assert.AssertionError);
  assert.match(error.message, /Missing expected rejection/);
  assert.equal(run.uploads.length, 1);
  assert.deepEqual(
    run.restored.slice(-restorationSteps.length),
    restorationSteps,
  );
});

test("the governance scenario logs and saves its canonical ID without losing the paid-refund result", async () => {
  const source = ts.createSourceFile(
    "testnet-scenarios.ts",
    await sourceText,
    ts.ScriptTarget.Latest,
    true,
  );
  let record = "";
  let governance = "";
  function visit(node: ts.Node) {
    if (ts.isFunctionDeclaration(node) && node.name?.text === "record")
      record = node.getText(source);
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(source) === "record" &&
      node.getText(source).includes('"governance-void-and-refund"')
    )
      governance = node.getText(source);
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.ok(record && governance);
  const id = testId(1, 1655n);
  const logged: unknown[] = [];
  const saved: unknown[] = [];
  await runInNewContext(
    ts.transpileModule(`(async () => { ${record}; await ${governance}; })()`, {
      compilerOptions: { target: ts.ScriptTarget.ES2022 },
    }).outputText,
    {
      results: [],
      ROOT: "/fake/run",
      join,
      sinkIdParts,
      governed: { id, tx: { transactionHash: "deposit" } },
      proposalId: 1n,
      relay: { akash: { state: async () => ({ height: 1667 }) } },
      console: { log: (value: string) => logged.push(JSON.parse(value)) },
      save: async (path: string, value: unknown) => {
        assert.equal(path, "/fake/run/scenarios-progress.json");
        saved.push(JSON.parse(JSON.stringify(value)));
      },
    },
  );
  const expected = {
    scenario: "governance-void-and-refund",
    passed: true,
    id,
    proposalId: "1",
    deposit: "deposit",
    refundConfirmedByHeight: 1667,
    refundedUakt: "5000000",
    ordinarySenderRejected: true,
  };
  assert.deepEqual(logged, [expected]);
  assert.deepEqual(saved, [[expected]]);
});

test("the final scenario accepts native Coin accounting and rejects unbalanced totals", async () => {
  const source = ts.createSourceFile(
    "testnet-scenarios.ts",
    await readFile(new URL("./testnet-scenarios.ts", import.meta.url), "utf8"),
    ts.ScriptTarget.Latest,
    true,
  );
  const assertions: string[] = [];
  function visit(node: ts.Node) {
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(source) === "assert.equal" &&
      /final\.totals\.(pendingAmount|mintedAmount)/.test(node.getText(source))
    )
      assertions.push(node.getText(source));
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.equal(assertions.length, 2);
  function check(pending: string, minted: string) {
    const final = QueryStateResponse.decode(
      QueryStateResponse.encode(
        QueryStateResponse.fromPartial({
          totals: {
            pendingAmount: { denom: "uakt", amount: pending },
            mintedAmount: { denom: "uakt", amount: minted },
          },
        }),
      ).finish(),
    );
    for (const statement of assertions)
      runInNewContext(statement, {
        assert,
        final,
        coinAmount,
        supply: 5_000_000n,
        burned: 2_000_000n,
      });
  }
  check("0", "7000000");
  assert.throws(() => check("1", "7000000"), assert.AssertionError);
  assert.throws(() => check("0", "6999999"), assert.AssertionError);
});
