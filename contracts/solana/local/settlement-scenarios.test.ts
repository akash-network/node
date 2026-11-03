import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { runInNewContext } from "node:vm";
import ts from "typescript";
import type { SinkId } from "../shared/id";
import { testId } from "../tests/sink-fixtures";
import { expectRefundError, refundCapScenario } from "./settlement-scenarios";

const refundErrors = {
  held: {
    code: 38,
    rawLog:
      "failed to execute message; message index: 0: settlement requires governance resolution",
  },
  ineligible: {
    code: 7,
    rawLog:
      "failed to execute message; message index: 0: refund needs current authorization without a hold: invalid settlement",
  },
  cap: {
    code: 21,
    rawLog:
      "failed to execute message; message index: 0: epoch refund cap reached",
  },
  immature: {
    code: 27,
    rawLog:
      "failed to execute message; message index: 0: challenge window has not elapsed",
  },
  ended: {
    code: 26,
    rawLog:
      "failed to execute message; message index: 0: challenge window ended",
  },
};

function refundChain(rollover?: "preparation" | "maturity") {
  let epoch = 57n;
  let time = 0n;
  let spent = 7_000_000n;
  let cap = 100_000_000n;
  let balance = 0n;
  const deadlines = new Map<SinkId, bigint>();
  const payments: { id: SinkId; epoch: bigint }[] = [];
  const rejections: { id: SinkId; epoch: bigint }[] = [];
  const caps: bigint[] = [];
  function advanceEpoch() {
    epoch++;
    spent = 0n;
  }
  const scenario: Parameters<typeof refundCapScenario>[0] = {
    prepare: async () => {
      const id = testId(deadlines.size + 1);
      const deadline = time + BigInt((deadlines.size + 1) * 100);
      deadlines.set(id, deadline);
      if (rollover === "preparation") advanceEpoch();
      return { id, challengeEnd: deadline };
    },
    state: async () => ({
      epoch,
      time,
      refundUsage: {
        epoch,
        amount: { denom: "uakt", amount: spent.toString() },
      },
    }),
    setCap: async (amount) => {
      cap = amount;
      caps.push(amount);
    },
    finalize: async (id, expectedError) => {
      const deadline = deadlines.get(id);
      assert.ok(
        deadline !== undefined && time >= deadline,
        "full challenge must mature",
      );
      const tx =
        spent + 2_000_000n > cap ? refundErrors.cap : { code: 0, rawLog: "" };
      if (tx.code) {
        rejections.push({ id, epoch });
      } else {
        spent += 2_000_000n;
        balance += 2_000_000n;
        payments.push({ id, epoch });
      }
      if (expectedError) await expectRefundError(async () => tx, expectedError);
      else assert.equal(tx.code, 0, tx.rawLog);
    },
    balance: async () => balance,
    sleep: async () => {
      time = [...deadlines.values()].reduce(
        (latest, deadline) => (deadline > latest ? deadline : latest),
        time,
      );
      if (rollover === "maturity") advanceEpoch();
    },
  };
  return { scenario, payments, rejections, caps, advanceEpoch };
}

test("refund cap rejects one mature candidate in the same epoch", async () => {
  const chain = refundChain();
  const result = await refundCapScenario(chain.scenario);
  assert.equal(result.epoch, 57n);
  assert.deepEqual(result.refunds, [testId(1), testId(2)]);
  assert.deepEqual(chain.payments, [{ id: testId(1), epoch: 57n }]);
  assert.deepEqual(chain.rejections, [{ id: testId(2), epoch: 57n }]);
  assert.deepEqual(chain.caps, [9_000_000n]);
});

for (const rollover of ["preparation", "maturity"] as const) {
  test(`refund cap uses fresh consumption after epoch rollover during ${rollover}`, async () => {
    const chain = refundChain(rollover);
    await refundCapScenario(chain.scenario);
    assert.deepEqual(chain.caps, [2_000_000n]);
    assert.equal(chain.payments.length, 1);
    assert.equal(chain.rejections.length, 1);
    assert.equal(chain.payments[0].epoch, chain.rejections[0].epoch);
  });
}

test("epoch rollover while setting the cap reports coverage inconclusive before paying", async () => {
  const chain = refundChain();
  const setCap = chain.scenario.setCap;
  chain.scenario.setCap = async (amount) => {
    await setCap(amount);
    chain.advanceEpoch();
  };
  await assert.rejects(
    refundCapScenario(chain.scenario),
    /coverage inconclusive/,
  );
  assert.deepEqual(chain.payments, []);
  assert.deepEqual(chain.rejections, []);
});

for (const sequence of [1, 2]) {
  test(`epoch rollover during refund ${sequence} reports coverage inconclusive`, async () => {
    const chain = refundChain();
    const finalize = chain.scenario.finalize;
    chain.scenario.finalize = async (id, expectedError) => {
      if (id === testId(sequence)) chain.advanceEpoch();
      await finalize(id, expectedError);
    };
    await assert.rejects(
      refundCapScenario(chain.scenario),
      /coverage inconclusive/,
    );
    assert.equal(chain.payments.length, sequence);
    assert.deepEqual(chain.rejections, []);
  });
}

test("a cap rejection followed by a boundary is not reported as same-epoch coverage", async () => {
  const chain = refundChain();
  const finalize = chain.scenario.finalize;
  chain.scenario.finalize = async (id, expectedError) => {
    await finalize(id, expectedError);
    if (id === testId(2)) chain.advanceEpoch();
  };
  await assert.rejects(
    refundCapScenario(chain.scenario),
    /coverage inconclusive/,
  );
  assert.equal(chain.rejections.length, 1);
});

test("two same-epoch payments cannot pass the cap scenario", async () => {
  const chain = refundChain();
  const setCap = chain.scenario.setCap;
  chain.scenario.setCap = (amount) => setCap(amount + 2_000_000n);
  await assert.rejects(
    refundCapScenario(chain.scenario),
    assert.AssertionError,
  );
  assert.equal(chain.payments.length, 2);
  assert.deepEqual(chain.rejections, []);
});

test("a non-cap transaction rejection cannot masquerade as a cap rejection", async () => {
  await assert.rejects(
    expectRefundError(
      async () => refundErrors.immature,
      /epoch refund cap reached/,
    ),
  );
});

test("refund rejection checks accept a returned cap error", async () => {
  await expectRefundError(
    async () => refundErrors.cap,
    /epoch refund cap reached/,
  );
});

test("refund rejection checks accept a thrown cap error", async () => {
  await expectRefundError(async () => {
    throw new Error(refundErrors.cap.rawLog);
  }, /epoch refund cap reached/);
});

test("refund rejection checks propagate unrelated broadcast failures", async () => {
  await assert.rejects(
    expectRefundError(async () => {
      throw new Error("RPC unavailable");
    }, /epoch refund cap reached/),
  );
});

test("a successful transaction cannot masquerade as a cap rejection", async () => {
  await assert.rejects(
    expectRefundError(
      async () => ({ code: 0, rawLog: "" }),
      /epoch refund cap reached/,
    ),
  );
});

test("held and immature refunds require their specific rejection in the actual scenarios", async () => {
  const source = ts.createSourceFile(
    "settlement-scenarios.ts",
    await readFile(
      new URL("./settlement-scenarios.ts", import.meta.url),
      "utf8",
    ),
    ts.ScriptTarget.Latest,
    true,
  );
  const calls: ts.CallExpression[] = [];
  function visit(node: ts.Node) {
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(source) === "finalize" &&
      node.arguments.length === 2 &&
      ts.isRegularExpressionLiteral(node.arguments[1])
    )
      calls.push(node);
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.equal(calls.length, 3);
  for (const call of calls) {
    const name = call.arguments[0].getText(source);
    assert.ok(name === "vetoed" || name === "offline" || name === "id");
    const expected =
      name === "id"
        ? refundErrors.immature
        : name === "vetoed"
          ? refundErrors.held
          : refundErrors.ineligible;
    const id = testId();
    for (const thrown of [false, true]) {
      const check = (tx: { code: number; rawLog: string }) =>
        runInNewContext(`(async () => { await ${call.getText(source)}; })()`, {
          [name]: id,
          finalize: async (actualId: SinkId, expectedError: RegExp) => {
            assert.equal(actualId, id);
            await expectRefundError(async () => {
              if (thrown) throw new Error(tx.rawLog);
              return tx;
            }, expectedError);
          },
        });
      await check(expected);
      for (const other of Object.values(refundErrors)) {
        if (other !== expected)
          await assert.rejects(check(other), assert.AssertionError);
      }
      await assert.rejects(
        check({ code: 0, rawLog: "" }),
        assert.AssertionError,
      );
      await assert.rejects(
        check({ code: 1, rawLog: "RPC unavailable" }),
        assert.AssertionError,
      );
    }
  }
});

test("a drained payer can be absent without accepting an unrelated relay failure", async () => {
  const source = ts.createSourceFile(
    "settlement-scenarios.ts",
    await readFile(
      new URL("./settlement-scenarios.ts", import.meta.url),
      "utf8",
    ),
    ts.ScriptTarget.Latest,
    true,
  );
  const calls: ts.CallExpression[] = [];
  function visit(node: ts.Node) {
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(source) === "assert.rejects" &&
      node.arguments[0]
        ?.getText(source)
        .includes("relay.relay(unfunded, proofHeight)")
    )
      calls.push(node);
    ts.forEachChild(node, visit);
  }
  visit(source);
  assert.equal(calls.length, 1);
  const check = (error?: Error) =>
    runInNewContext(`(async () => { await ${calls[0].getText(source)}; })()`, {
      assert,
      unfunded: testId(),
      proofHeight: 10,
      relay: {
        relay: async () => {
          if (error) throw error;
        },
      },
    });
  await check(
    new Error(
      "proof-create: simulation failed\n+ actual - expected\n\n+ 'AccountNotFound'\n- null\n",
    ),
  );
  await check(new Error("insufficient lamports"));
  await assert.rejects(
    check(new Error("RPC unavailable")),
    assert.AssertionError,
  );
  await assert.rejects(
    check(new Error("status-fetch: AccountNotFound")),
    assert.AssertionError,
  );
  await assert.rejects(check(), assert.AssertionError);
});
