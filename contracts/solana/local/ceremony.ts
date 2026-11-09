import assert from "node:assert/strict";
import { web3 } from "@coral-xyz/anchor";
import { z } from "zod";
import type { checkRelease } from "../deploy/programs";
import * as ix from "./admin-instructions";

export type LaunchManifest = Awaited<
  ReturnType<typeof checkRelease>
>["manifest"];

export function handoverInstructions(manifest: LaunchManifest) {
  const ceremony = new web3.PublicKey(manifest.ceremonyAuthority);
  const vault = new web3.PublicKey(manifest.upgradeAuthority);
  return [
    ...ix.handoverAccessManager(
      ix.accessManager(new web3.PublicKey(manifest.programs.access_manager)),
      ceremony,
      vault,
    ),
    ...Object.values(manifest.programs).map((program) =>
      ix.setUpgradeAuthority(new web3.PublicKey(program), ceremony, vault),
    ),
  ];
}

function equalInstruction(
  actual: web3.TransactionInstruction,
  expected: web3.TransactionInstruction,
) {
  return (
    actual.programId.equals(expected.programId) &&
    actual.data.equals(expected.data) &&
    actual.keys.length === expected.keys.length &&
    actual.keys.every((key, index) => {
      const wanted = expected.keys[index];
      return (
        key.pubkey.equals(wanted.pubkey) &&
        key.isSigner === wanted.isSigner &&
        key.isWritable === wanted.isWritable
      );
    })
  );
}

// Accept only initial setup or the complete, atomic authority handover.
// Instruction names come from decoding the bytes, never from the command label.
export function ceremonyBundle(
  value: unknown,
  manifest: LaunchManifest,
  genesis: string,
) {
  const { file, authority, instructions } = ix.readEmitFile(value);
  assert.equal(file.genesisHash, genesis, "bundle names another cluster");
  assert.equal(
    authority.toBase58(),
    manifest.ceremonyAuthority,
    "bundle authority differs from the ceremony manifest",
  );
  const handover = handoverInstructions(manifest);
  if (
    instructions.length === handover.length &&
    instructions.every((entry, index) =>
      equalInstruction(entry, handover[index]),
    )
  )
    return { file, instructions, phase: "handover" as const };

  let clientAt = -1;
  let bridgeAt = -1;
  for (const [index, entry] of file.summary.entries()) {
    assert.ok(
      entry.program !== "bpf_loader_upgradeable",
      "setup cannot change code or authorities",
    );
    const name = z
      .enum(["akt", "ics07_tendermint", "access_manager"])
      .parse(entry.program);
    assert.equal(
      entry.programId,
      manifest.programs[name],
      "instruction targets a program outside the release",
    );
    if (entry.program === "akt") {
      assert.ok(
        [
          "set_token_metadata",
          "set_pause_guardian",
          "configure_bridge",
        ].includes(entry.instruction),
        "ceremony cannot activate minting or perform runtime administration",
      );
      if (entry.instruction === "configure_bridge") {
        assert.equal(bridgeAt, -1, "duplicate bridge configuration");
        bridgeAt = index;
        assert.equal(
          entry.accounts.find(
            (account) => account.name === "light_client_program",
          )?.pubkey,
          manifest.programs.ics07_tendermint,
          "bridge must pin the release light client",
        );
      }
    } else if (entry.program === "ics07_tendermint") {
      assert.equal(
        entry.instruction,
        "initialize",
        "only initial client setup is allowed",
      );
      assert.equal(clientAt, -1, "duplicate client initialization");
      clientAt = index;
      const args = z.object({ access_manager: z.string() }).parse(entry.args);
      assert.equal(
        args.access_manager,
        manifest.programs.access_manager,
        "wrong access manager",
      );
    } else {
      assert.equal(
        entry.instruction,
        "grant_role",
        "setup may only grant relayer membership",
      );
      z.object({ role_id: z.literal("1") }).parse(entry.args);
    }
  }
  if (clientAt !== -1 || bridgeAt !== -1)
    assert.ok(
      clientAt !== -1 && bridgeAt > clientAt,
      "initialize client and configure bridge together, in that order",
    );
  return { file, instructions, phase: "setup" as const };
}
