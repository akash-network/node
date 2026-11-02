import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { promisify } from "node:util";
import { toBech32 } from "@cosmjs/encoding";
import { web3 } from "@coral-xyz/anchor";
import example from "./config.example.json";
import { parseConfig } from "./config";

// Port 9 keeps a CLI test from reaching any node, including a tunnel on 8899.
function offline() {
  return {
    ...example,
    akashRpc: "http://127.0.0.1:9",
    solanaRpc: "http://127.0.0.1:9",
    reporter: toBech32("akash", new Uint8Array(20).fill(7)),
    solanaGenesisHash: web3.Keypair.generate().publicKey.toBase58(),
  };
}

async function cli(config: string, flags: string[]) {
  return promisify(execFile)(
    process.execPath,
    [
      "--import",
      "tsx",
      join(__dirname, "run.ts"),
      "--config",
      config,
      ...flags,
    ],
    { timeout: 10_000 },
  );
}

test("config accepts an optional bounded maxSlotLead", () => {
  const base = {
    ...example,
    reporter: toBech32("akash", new Uint8Array(20).fill(7)),
    solanaGenesisHash: web3.Keypair.generate().publicKey.toBase58(),
  };
  assert.equal(parseConfig(base).maxSlotLead, undefined);
  assert.equal(parseConfig({ ...base, maxSlotLead: 1200 }).maxSlotLead, 1200);
  for (const maxSlotLead of [0, 100_001, 1.5, "600"])
    assert.throws(
      () => parseConfig({ ...base, maxSlotLead }),
      /invalid maxSlotLead/,
    );
});

test("config pins a light client only when one is set", () => {
  const base = {
    ...example,
    reporter: toBech32("akash", new Uint8Array(20).fill(7)),
    solanaGenesisHash: web3.Keypair.generate().publicKey.toBase58(),
  };
  assert.ok(!("lightClientProgram" in example));
  assert.equal(parseConfig(base).lightClientProgram, undefined);
  const pinned = web3.Keypair.generate().publicKey;
  assert.ok(
    parseConfig({
      ...base,
      lightClientProgram: pinned.toBase58(),
    }).lightClientProgram?.equals(pinned),
  );
  for (const lightClientProgram of ["", 7, `0${pinned.toBase58()}`])
    assert.throws(() => parseConfig({ ...base, lightClientProgram }));
});

test("CLI refuses implicit signing and never echoes malformed private key contents", async () => {
  const directory = await mkdtemp(join(tmpdir(), "akt-reporter-test-"));
  try {
    const config = join(directory, "config.json");
    const key = join(directory, "reporter.key");
    const secret = "DO_NOT_PRINT_THIS_PRIVATE_KEY_FILE_CONTENT";
    await writeFile(config, JSON.stringify(offline()));
    await writeFile(key, secret, { mode: 0o600 });
    const run = promisify(execFile);
    for (const flags of [
      ["--key-file", key],
      ["--broadcast", "--key-file", key],
    ]) {
      await assert.rejects(
        () =>
          run(
            process.execPath,
            [
              "--import",
              "tsx",
              join(__dirname, "run.ts"),
              "--config",
              config,
              ...flags,
            ],
            { timeout: 10_000 },
          ),
        (error: unknown) => {
          assert.ok(
            error instanceof Error &&
              "stderr" in error &&
              typeof error.stderr === "string",
          );
          assert.ok(
            !error.message.includes(secret) && !error.stderr.includes(secret),
            "secret leaked through validation error",
          );
          assert.match(
            error.stderr,
            flags.includes("--broadcast")
              ? /key file must contain/
              : /preview never loads keys/,
          );
          return true;
        },
      );
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("CLI runs an external signer only for broadcast and never beside a key file", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "akt-reporter-test-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const config = join(directory, "config.json");
  const marker = join(directory, "ran");
  const command = join(directory, "signer.sh");
  await writeFile(command, `#!/bin/sh\ntouch '${marker}'\n`, { mode: 0o700 });
  await writeFile(config, JSON.stringify(offline()));
  const cases: [string[], RegExp][] = [
    [["--signer-command", command], /preview never loads keys/],
    [["--broadcast"], /exactly one of --key-file or an external signer/],
    [
      ["--broadcast", "--key-file", config, "--signer-command", command],
      /exactly one of --key-file or an external signer/,
    ],
    [["--broadcast", "--signer-command", "signer.sh"], /absolute path/],
  ];
  for (const [flags, error] of cases)
    await assert.rejects(
      () => cli(config, flags),
      (failure: unknown) => {
        assert.ok(
          failure instanceof Error &&
            "stderr" in failure &&
            typeof failure.stderr === "string",
        );
        assert.match(failure.stderr, error);
        return true;
      },
    );
  await writeFile(
    config,
    JSON.stringify({ ...offline(), externalSigner: { command } }),
  );
  await assert.rejects(
    () => cli(config, ["--broadcast", "--key-file", config]),
    /exactly one of --key-file or an external signer/,
  );
  await assert.rejects(() => stat(marker), { code: "ENOENT" });
});
