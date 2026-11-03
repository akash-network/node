import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { runInNewContext } from "node:vm";
import test from "node:test";
import ts from "typescript";
import { QueryStateResponse } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { coinAmount } from "../shared/proof";

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
