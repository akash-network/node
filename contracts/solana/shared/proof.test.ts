import { web3 } from "@coral-xyz/anchor";
import {
  RecordData,
  SettlementData,
  Sink_Status as Status,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import assert from "node:assert/strict";
import test from "node:test";
import boundaries from "../programs/akt/tests/fixtures/native_boundary_vectors.json";
import vectors from "../programs/akt/tests/fixtures/proof_vectors.json";
import settlements from "../programs/akt/tests/fixtures/settlement_vectors.json";
import { MAX_INT64, MAX_UINT64, parseSinkId, sinkIdParts } from "./id";
import {
  authorizedSettlement,
  decodeRecordData,
  encodeRecord,
  proofAddress,
  proofNonce,
  settlementKey,
  settlementProofValue,
  statusAddress,
  transferKey,
} from "./proof";

for (const vector of [...vectors, ...boundaries.records])
  test(`SDK native RecordData vector ${vector.name}`, () => {
    const id = parseSinkId({
      owner: vector.owner,
      height: vector.height,
      sequence: vector.sequence,
    });
    assert.equal(id, vector.id_text);
    assert.equal(
      Buffer.from(transferKey(id)).toString("hex"),
      vector.transfer_key_hex,
    );
    const data = {
      dest: vector.recipient,
      amount: { denom: "uakt", amount: vector.amount },
    };
    const bytes = encodeRecord(data);
    assert.equal(bytes.toString("hex"), vector.record_value_hex);
    assert.deepEqual(RecordData.decode(bytes), data);
    assert.equal(decodeRecordData(bytes).amount.toString(), vector.amount);
    assert.equal(
      proofNonce(transferKey(id)).toString("hex"),
      vector.mint_proof_nonce_hex,
    );
    const program = web3.Keypair.generate().publicKey;
    const expected = web3.PublicKey.findProgramAddressSync(
      [
        Buffer.from("transfer"),
        proofNonce(Buffer.from(vector.owner_hex, "hex")),
        Buffer.from(vector.height_le_hex, "hex"),
        Buffer.from(vector.sequence_le_hex, "hex"),
      ],
      program,
    )[0];
    assert.ok(statusAddress(program, id).equals(expected));
  });

for (const vector of [...settlements, ...boundaries.settlements])
  test(`SDK native SettlementData vector ${vector.name}`, () => {
    const id = parseSinkId({
      owner: vector.owner,
      height: vector.height,
      sequence: vector.sequence,
    });
    const settlement = SettlementData.fromJSON(vector.settlement);
    const transfer = TransferState.fromPartial({
      id: sinkIdParts(id),
      status: vector.status,
      settlement,
    });
    const key = settlementKey(id, vector.status);
    assert.equal(Buffer.from(key).toString("hex"), vector.settlement_key_hex);
    assert.equal(proofNonce(key).toString("hex"), vector.proof_nonce_hex);
    const value = Buffer.from(vector.settlement_value_hex, "hex");
    assert.deepEqual(
      Buffer.from(SettlementData.encode(settlement).finish()),
      value,
    );
    const height =
      settlement.voidAuthorization?.requestedHeight ?? BigInt(vector.height);
    if (settlement.voidAuthorization) {
      assert.deepEqual(settlementProofValue(transfer, height, value), value);
      assert.throws(
        () =>
          settlementProofValue(
            transfer,
            height,
            Buffer.concat([value, Buffer.from([0x48, 0])]),
          ),
        /noncanonical/,
      );
      assert.throws(
        () => authorizedSettlement(transfer, height - 1n),
        /height or round/,
      );
      assert.throws(
        () =>
          authorizedSettlement(
            { ...transfer, settlement: { ...settlement, round: 0n } },
            height,
          ),
        /height or round/,
      );
      assert.throws(
        () =>
          settlementProofValue(
            {
              ...transfer,
              settlement: {
                ...settlement,
                vetoReason: settlement.vetoReason + "changed",
              },
            },
            height,
            value,
          ),
        /differs from snapshot/,
      );
    } else
      assert.throws(
        () => authorizedSettlement(transfer, height),
        /missing permanent/,
      );
  });

test("every status has a distinct full-key nonce and buffer while sharing one outcome PDA", () => {
  const id = parseSinkId(vectors[0].id_text),
    program = web3.Keypair.generate().publicKey,
    payer = web3.Keypair.generate().publicKey;
  const keys = [
    transferKey(id),
    ...[1, 2, 3, 4, 5].map((status) => settlementKey(id, status)),
  ];
  assert.equal(
    new Set(keys.map((key) => proofNonce(key).toString("hex"))).size,
    6,
  );
  assert.equal(
    new Set(keys.map((key) => proofAddress(program, payer, key).toBase58()))
      .size,
    6,
  );
  for (const status of [0, -1, 6, 1.5])
    assert.throws(() => settlementKey(id, status));
  assert.equal(settlementKey(id, Status.STATUS_VOIDED).length, 40);
});

test("native record validation rejects legacy, ambiguous and noncanonical values", () => {
  const vector = vectors[0];
  for (const value of [
    "1",
    1n,
    `${vector.owner}/0/1`,
    `${vector.owner}/01/1`,
    `${vector.owner}/${MAX_INT64 + 1n}/1`,
    `${vector.owner}/1/${MAX_UINT64 + 1n}`,
    { owner: vector.owner, height: 1, sequence: 1n },
  ])
    assert.throws(() => parseSinkId(value));
  const good = encodeRecord({
    dest: vector.recipient,
    amount: { denom: "uakt", amount: vector.amount },
  });
  for (const bytes of [
    Buffer.alloc(41),
    Buffer.alloc(77),
    Buffer.concat([good, good]),
    Buffer.concat([good, Buffer.from([0x18, 0])]),
  ])
    assert.throws(() => decodeRecordData(bytes));
  for (const amount of ["0", "01", "-1", (MAX_UINT64 + 1n).toString()])
    assert.throws(() =>
      encodeRecord({
        dest: vector.recipient,
        amount: { denom: "uakt", amount },
      }),
    );
});
