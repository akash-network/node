import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { isAbsolute, resolve } from "node:path";
import { parseArgs } from "node:util";
import { z } from "zod";
import { createKey, keyAddress } from "./keys";
import { checkServiceConfig } from "./service-config";

async function main() {
  const role = z.enum(["relayer", "reporter"]).parse(process.argv[2]);
  const args = process.argv.slice(3);
  const { values, positionals } = parseArgs({
    args,
    allowPositionals: true,
    options: {
      help: { type: "boolean" },
      version: { type: "boolean" },
      network: { type: "string", default: "mainnet" },
      config: { type: "string" },
      out: { type: "string" },
      "key-file": { type: "string" },
      broadcast: { type: "boolean", default: false },
      watch: { type: "boolean", default: false },
      "reset-budget": { type: "string" },
    },
  });
  const network = z.enum(["mainnet", "testnet"]).parse(values.network);
  if (values.help || args.length === 0) {
    process.stdout.write(
      `akt-${role} run --config /absolute/config.json [--watch]${role === "reporter" ? " [--broadcast]" : ""}\n` +
        `akt-${role} check --config /absolute/config.json${role === "reporter" ? " [--broadcast]" : ""}\n` +
        `akt-${role} keys create --out /absolute/key-file\n` +
        `akt-${role} keys address --key-file /absolute/key-file\n` +
        (role === "relayer"
          ? `akt-relayer run --config /absolute/config.json --reset-budget OWNER/HEIGHT/SEQUENCE|all\n`
          : "") +
        `akt-${role} --version\n` +
        `run/check: --network testnet opts into isolated test networks (default: mainnet).\n`,
    );
    return;
  }
  if (values.version) {
    process.stdout.write(
      await readFile(
        resolve(__dirname, "../../../service-build-info.txt"),
        "utf8",
      ),
    );
    return;
  }
  if (positionals[0] === "keys") {
    assert.equal(positionals.length, 2, "expected keys create or keys address");
    assert.ok(
      !values.config &&
        !values.broadcast &&
        !values.watch &&
        !values["reset-budget"],
      "unexpected service flags for keys",
    );
    if (positionals[1] === "create") {
      assert.ok(
        values.out && !values["key-file"],
        "keys create requires --out",
      );
      process.stdout.write(`${await createKey(role, values.out)}\n`);
    } else {
      assert.ok(
        positionals[1] === "address" && values["key-file"] && !values.out,
        "keys address requires --key-file",
      );
      process.stdout.write(`${await keyAddress(role, values["key-file"])}\n`);
    }
    return;
  }
  const command = z.enum(["run", "check"]).parse(positionals[0]);
  assert.equal(positionals.length, 1, "expected one service command");
  assert.ok(
    values.config && isAbsolute(values.config),
    "--config must be an absolute path",
  );
  assert.ok(
    !values.out && !values["key-file"],
    "set the signing key path in config.json",
  );
  assert.ok(
    role === "reporter" || !values.broadcast,
    "relayer always submits proofs; --broadcast is reporter-only",
  );
  const service =
    role === "relayer"
      ? role
      : values.broadcast
        ? "reporter-broadcast"
        : "reporter-preview";
  checkServiceConfig(
    service,
    JSON.parse(await readFile(values.config, "utf8")),
    values.config,
    network,
  );
  assert.ok(
    !values["reset-budget"] ||
      (role === "relayer" && command === "run" && !values.watch),
    "budget reset requires a one-shot relayer run",
  );
  if (command === "check") {
    assert.ok(!values.watch, "--watch is only for run");
    process.stdout.write(
      "Configuration valid. No RPC request or transaction sent.\n",
    );
    return;
  }
  const flags = ["--config", values.config];
  if (values.watch) flags.push("--watch");
  if (values["reset-budget"])
    flags.push("--reset-budget", values["reset-budget"]);
  if (role === "reporter") {
    if (values.broadcast) flags.push("--broadcast");
    const { runReporter } = await import("../reporter/run");
    await runReporter(flags);
  } else {
    const { runRelayer } = await import("../local/relay");
    await runRelayer(flags);
  }
}

void main().catch((error: unknown) => {
  process.stderr.write(
    `${error instanceof Error ? error.message : "operator command failed"}\n`,
  );
  process.exitCode = 1;
});
