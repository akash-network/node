import assert from "node:assert/strict";
import { readFileSync, writeFileSync } from "node:fs";
import { web3 } from "@coral-xyz/anchor";

// scripts/test-contract.sh deploys more copies of the pinned verifier here,
// so re-pin tests have fresh client PDAs.
export const REPIN_CLIENT = new web3.PublicKey(
  "5zZkzMbXxS5n9LMwKeKD84GL6S8PmCJKQQDa9jmNsq8A",
);
// The tests initialize this copy with a frozen checkpoint.
export const FROZEN_CLIENT = new web3.PublicKey(
  "5KEdJk221E5GuQHVMwsvf7tUQviHrUBvKGaUioDZYhyB",
);
const TARGETS = new Map([
  ["repin", REPIN_CLIENT],
  ["frozen", FROZEN_CLIENT],
]);
export const CLIENT = new web3.PublicKey(
  "HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD",
);

// Anchor programs reject calls at any address other than their declared ID.
// The compiled program holds that ID as one 32-byte constant and as 64-bit
// lddw immediates: [0x18, regs, offset16, low32][0x00, 0, 0, high32].
export function relocate(
  binary: Buffer,
  from: web3.PublicKey,
  to: web3.PublicKey,
) {
  const output = Buffer.from(binary);
  const source = from.toBuffer();
  const target = to.toBuffer();
  let constants = 0;
  let immediates = 0;
  for (let at = output.indexOf(source); at >= 0; at = output.indexOf(source)) {
    target.copy(output, at);
    constants++;
  }
  for (let word = 0; word < 32; word += 8) {
    const low = source.subarray(word, word + 4);
    const high = source.subarray(word + 4, word + 8);
    for (
      let at = output.indexOf(low);
      at >= 0;
      at = output.indexOf(low, at + 1)
    ) {
      if (
        at >= 4 &&
        output[at - 4] === 0x18 &&
        output.readUInt32LE(at + 4) === 0 &&
        output.subarray(at + 8, at + 12).equals(high)
      ) {
        target.copy(output, at, word, word + 4);
        target.copy(output, at + 8, word + 4, word + 8);
        immediates++;
      }
    }
    assert.ok(
      !output.includes(low) && !output.includes(high),
      "unrecognized program ID encoding",
    );
  }
  assert.ok(constants > 0 && immediates > 0, "program ID not found");
  return output;
}

if (require.main === module) {
  const [input, output, name = "repin"] = process.argv.slice(2);
  const target = TARGETS.get(name);
  assert.ok(
    input && output && target,
    "usage: relocate-program.ts INPUT OUTPUT [repin|frozen]",
  );
  writeFileSync(output, relocate(readFileSync(input), CLIENT, target));
  console.log(target.toBase58());
}
