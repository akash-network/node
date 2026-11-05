import assert from "node:assert/strict";
import test from "node:test";
import { DestType } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { sinkAminoTypes, sinkMessage, sinkRegistry } from "./sink";

test("MsgSink Amino matches the node's sign bytes and accepts only a Solana destination", () => {
  const amino = sinkAminoTypes(),
    message = sinkMessage({
      sender: "akash1qurswpc8qurswpc8qurswpc8qurswpc87g2aem",
      recipient: "US517G5965aydkZ46HS38QLi7UQiSojurfbQfKCELFx",
      amountUakt: "1000000",
    }),
    converted = amino.toAmino(message);
  assert.equal(message.value.destType, DestType.DEST_TYPE_SOLANA);
  assert.equal(
    JSON.stringify(converted, [
      "amount",
      "denom",
      "dest",
      "dest_type",
      "sender",
      "type",
      "value",
    ]),
    '{"type":"sink/MsgSink","value":{"amount":{"amount":"1000000","denom":"uakt"},"dest":"US517G5965aydkZ46HS38QLi7UQiSojurfbQfKCELFx","dest_type":1,"sender":"akash1qurswpc8qurswpc8qurswpc8qurswpc87g2aem"}}',
  );
  assert.deepEqual(
    sinkRegistry().decode({
      typeUrl: message.typeUrl,
      value: sinkRegistry().encode(amino.fromAmino(converted)),
    }),
    message.value,
  );
  for (const destType of [0, 2, "DEST_TYPE_SOLANA", undefined])
    assert.throws(
      () =>
        amino.fromAmino({
          ...converted,
          value: { ...converted.value, dest_type: destType },
        }),
      /Amino fields/,
    );
});
