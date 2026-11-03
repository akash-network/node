import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import test from "node:test";
import { BN, BorshCoder, web3 } from "@coral-xyz/anchor";
import generatedIdl from "../../../.cache/solana/target/idl/akt_solana.json";
import vectors from "../programs/akt-solana/tests/fixtures/proof_vectors.json";
import { assertCanonicalIdl } from "./idl.test";

const idl: unknown = generatedIdl;
assertCanonicalIdl(idl);
const coder = new BorshCoder(idl);
const program = new web3.PublicKey(idl.address);
const payer = new web3.PublicKey(Buffer.alloc(32, 0x33));
const hex = (value: string) => Buffer.from(value, "hex");
const observable = (value: unknown): unknown =>
  JSON.parse(JSON.stringify(value));
const sha256 = (key: Buffer) => createHash("sha256").update(key).digest();

for (const vector of vectors) {
  test(`program Borsh encoding matches SDK vector ${vector.name}`, async () => {
    const id = {
      owner: [...hex(vector.owner_hex)],
      height: new BN(vector.height),
      sequence: new BN(vector.sequence),
    };
    const record = {
      recipient: new web3.PublicKey(vector.recipient),
      amount: new BN(vector.amount),
    };
    const heightLE = id.height.toArrayLike(Buffer, "le", 8);
    const sequenceLE = id.sequence.toArrayLike(Buffer, "le", 8);
    assert.deepEqual(heightLE, hex(vector.height_le_hex));
    assert.deepEqual(sequenceLE, hex(vector.sequence_le_hex));
    assert.deepEqual(record.recipient.toBuffer(), hex(vector.recipient_hex));
    const destination = Buffer.from(vector.recipient);
    const decimal = Buffer.from(vector.amount);
    const nativeRecord = Buffer.concat([
      Buffer.from([10, destination.length]),
      destination,
      Buffer.from([18, decimal.length + 8, 10, 4]),
      Buffer.from("uakt"),
      Buffer.from([18, decimal.length]),
      decimal,
    ]);
    assert.deepEqual(nativeRecord, hex(vector.record_value_hex));
    const idBytes = Buffer.concat([
      hex(vector.owner_hex),
      heightLE,
      sequenceLE,
    ]);
    assert.deepEqual(coder.types.encode("TransferId", id), idBytes);

    const transferKey = Buffer.concat([
      Buffer.from([0x02, 0, 20]),
      hex(vector.owner_hex),
      id.height.toArrayLike(Buffer, "be", 8),
      id.sequence.toArrayLike(Buffer, "be", 8),
    ]);
    const settlementKey = Buffer.concat([
      Buffer.from([3, 1, 3]),
      transferKey.subarray(2),
    ]);
    assert.deepEqual(transferKey, hex(vector.transfer_key_hex));
    const mintNonce = sha256(transferKey);
    const voidNonce = sha256(settlementKey);
    assert.deepEqual(mintNonce, hex(vector.mint_proof_nonce_hex));
    assert.notDeepEqual(mintNonce, voidNonce);

    const proofHeight = id.height.addn(1);
    const mint = coder.instruction.encode("mint_transfer", {
      id,
      recipient_key: record.recipient,
      amount: record.amount,
      proof_height: proofHeight,
    });
    assert.equal(mint.length, 92);
    assert.deepEqual(
      mint.subarray(8),
      Buffer.concat([
        idBytes,
        record.recipient.toBuffer(),
        record.amount.toArrayLike(Buffer, "le", 8),
        proofHeight.toArrayLike(Buffer, "le", 8),
      ]),
    );
    const voidInstruction = coder.instruction.encode("void_transfer", {
      id,
      settlement_status: 3,
      proof_height: proofHeight,
    });
    assert.equal(voidInstruction.length, 53);
    assert.deepEqual(
      voidInstruction.subarray(8),
      Buffer.concat([
        idBytes,
        Buffer.from([3]),
        proofHeight.toArrayLike(Buffer, "le", 8),
      ]),
    );
    for (const nonce of [mintNonce, voidNonce]) {
      const create = coder.instruction.encode("create_proof_buffer", {
        nonce: [...nonce],
        expected_len: 16384,
      });
      assert.equal(create.length, 44);
      assert.deepEqual(create.subarray(8, 40), nonce);
      assert.equal(create.readUInt32LE(40), 16384);
    }

    for (const outcome of [
      { Minted: { amount: record.amount, recipient: record.recipient } },
      { Voided: { reason: { InvalidRecipient: {} } } },
      { Voided: { reason: { Governance: {} } } },
    ]) {
      const evidence = {
        id,
        proof_height: proofHeight,
        resolved_at_slot: new BN(99),
        outcome,
      };
      const status = await coder.accounts.encode("TransferStatus", evidence);
      assert.ok(status.length <= 101);
      assert.deepEqual(
        observable(coder.accounts.decode("TransferStatus", status)),
        observable(evidence),
      );
      const event = idl.events?.find(
        (entry) => entry.name === "TransferResolved",
      );
      assert.ok(event);
      const eventBytes = Buffer.concat([
        Buffer.from(event.discriminator),
        coder.types.encode("TransferResolved", evidence),
      ]);
      assert.deepEqual(
        observable(coder.events.decode(eventBytes.toString("base64"))),
        observable({ name: "TransferResolved", data: evidence }),
      );
    }

    const status = web3.PublicKey.findProgramAddressSync(
      [Buffer.from("transfer"), hex(vector.owner_hex), heightLE, sequenceLE],
      program,
    )[0];
    const otherStatus = web3.PublicKey.findProgramAddressSync(
      [Buffer.from("transfer"), Buffer.alloc(20, 0x55), heightLE, sequenceLE],
      program,
    )[0];
    assert.ok(!status.equals(otherStatus));
    const buffer = (submitter: web3.PublicKey, nonce: Buffer) =>
      web3.PublicKey.findProgramAddressSync(
        [Buffer.from("proof"), submitter.toBuffer(), nonce],
        program,
      )[0];
    assert.ok(!buffer(payer, mintNonce).equals(buffer(payer, voidNonce)));
    assert.ok(!buffer(payer, mintNonce).equals(buffer(program, mintNonce)));
    const changedTail = Buffer.from(mintNonce);
    changedTail[31] ^= 1;
    assert.ok(!buffer(payer, mintNonce).equals(buffer(payer, changedTail)));
  });
}

test("grow instruction carries no caller-controlled allocation size", () => {
  const grow = coder.instruction.encode("grow_proof_buffer", {});
  assert.equal(grow.length, 8);
});
