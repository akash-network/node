import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { Program } from "@coral-xyz/anchor";
import { z } from "zod";
import idl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { AkashClient } from "../reporter/akash";
import type { SinkId } from "../shared/id";
import {
  AKASH_RPC,
  CHAIN,
  GO_RELAYER,
  PROGRAM_ID,
  REPO,
  provider,
} from "./common";
import {
  clientProgram,
  discoverLightClient,
  lightClientAccounts,
} from "./light-client";

const exec = promisify(execFile);
const relayResult = z.object({
  event: z.literal("relay-result"),
  result: z.enum(["resolved", "already-resolved", "skipped"]),
});

// Scenarios retain their readonly inspectors and admin controls. Every relayer
// action runs the same Go binary as the local watcher and release package.
export async function relayContext() {
  const wallet = await provider();
  const program = new Program<Akt>(
    { ...idl, address: PROGRAM_ID.toBase58() },
    wallet,
  );
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  async function command(args: string[]) {
    const result = await exec(GO_RELAYER, ["local", ...args], {
      cwd: REPO,
      timeout: 600_000,
      maxBuffer: 8 * 1024 * 1024,
    });
    process.stdout.write(result.stdout);
    process.stderr.write(result.stderr);
    return result.stdout;
  }
  return {
    wallet,
    program,
    akash,
    bootstrap: () => command(["bootstrap"]),
    tick: () => command(["tick"]),
    update: (height: number, tamper = false) =>
      command(["update", String(height), ...(tamper ? ["--tamper"] : [])]),
    relay: async (id: SinkId, height: number) => {
      const output = await command(["relay", id.toString(), String(height)]);
      const last = output.trimEnd().split("\n").at(-1);
      assert.ok(last, "Go relayer did not return a result");
      return relayResult.parse(JSON.parse(last)).result;
    },
    lightClient: async () => {
      const ids = await discoverLightClient(wallet.connection, PROGRAM_ID);
      assert.equal(ids.chainId, CHAIN);
      return {
        ...ids,
        ...lightClientAccounts(ids),
        client: clientProgram(wallet, ids.program),
      };
    },
    close: () => akash.close(),
  };
}
