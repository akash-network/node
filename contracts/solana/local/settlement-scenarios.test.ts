import assert from "node:assert/strict";
import test from "node:test";
import type { SinkId } from "../shared/id";
import { testId } from "../tests/sink-fixtures";
import { expectRefundError, refundCapScenario } from "./settlement-scenarios";

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
      const error = spent + 2_000_000n > cap ? "daily refund cap" : undefined;
      if (error) {
        rejections.push({ id, epoch });
      } else {
        spent += 2_000_000n;
        balance += 2_000_000n;
        payments.push({ id, epoch });
      }
      if (expectedError)
        assert.match(error ?? "refund succeeded", expectedError);
      else assert.equal(error, undefined);
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
  await assert.rejects(refundCapScenario(chain.scenario), /refund succeeded/);
  assert.equal(chain.payments.length, 2);
  assert.deepEqual(chain.rejections, []);
});

test("a non-cap transaction rejection cannot masquerade as a cap rejection", async () => {
  await assert.rejects(
    expectRefundError(
      async () => ({ code: 7, rawLog: "challenge window" }),
      /daily refund cap/,
    ),
  );
});

test("refund rejection checks accept a returned cap error", async () => {
  await expectRefundError(
    async () => ({ code: 7, rawLog: "daily refund cap" }),
    /daily refund cap/,
  );
});

test("refund rejection checks accept a thrown cap error", async () => {
  await expectRefundError(async () => {
    throw new Error("daily refund cap");
  }, /daily refund cap/);
});

test("refund rejection checks propagate unrelated broadcast failures", async () => {
  await assert.rejects(
    expectRefundError(async () => {
      throw new Error("RPC unavailable");
    }, /daily refund cap/),
  );
});

test("a successful transaction cannot masquerade as a cap rejection", async () => {
  await assert.rejects(
    expectRefundError(
      async () => ({ code: 0, rawLog: "" }),
      /daily refund cap/,
    ),
  );
});
