import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { join, resolve } from "node:path";
import { test, type TestContext } from "node:test";
import { promisify } from "node:util";
import { web3 } from "@coral-xyz/anchor";
import { checkRelease } from "./programs";
import { MAINNET_GENESIS } from "../shared/networks";

const exec = promisify(execFile);
const repo = resolve(__dirname, "..");
const names = ["access_manager", "ics07_tendermint", "akt"] as const;
const hash = (bytes: Buffer | string) =>
  createHash("sha256").update(bytes).digest("hex");

async function fixture(t: TestContext) {
  const cache = resolve(repo, "../../.cache/solana/tests");
  await mkdir(cache, { recursive: true });
  const root = await mkdtemp(join(cache, "deployment-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const build = join(root, "build");
  await mkdir(build);
  const programs = {
    access_manager: web3.Keypair.generate().publicKey.toBase58(),
    ics07_tendermint: web3.Keypair.generate().publicKey.toBase58(),
    akt: web3.Keypair.generate().publicKey.toBase58(),
  };
  let sums = "";
  for (const name of names) {
    const elf = Buffer.from("\x7fELFtest-program-" + name);
    const idl = JSON.stringify({ address: programs[name] });
    await writeFile(join(build, `${name}.so`), elf);
    await writeFile(join(build, `${name}.json`), idl);
    await writeFile(join(build, `${name}.ts`), "// generated interface\n");
    sums += `${hash("// generated interface\n")}  ${name}.ts\n`;
    sums += `${hash(elf)}  ${name}.so\n${hash(idl)}  ${name}.json\n`;
  }
  await writeFile(join(build, "SHA256SUMS"), sums);
  const manifest = {
    version: 1,
    sourceCommit: "a".repeat(40),
    buildDirectory: "./build",
    buildInfoSha256: hash(`commit ${"a".repeat(40)}\n`),
    checksumsSha256: hash(sums),
    solanaGenesisHash: MAINNET_GENESIS,
    ceremonyAuthority: web3.Keypair.generate().publicKey.toBase58(),
    upgradeAuthority: web3.Keypair.generate().publicKey.toBase58(),
    programs,
  };
  await writeFile(
    join(build, "build-info.txt"),
    `commit ${manifest.sourceCommit}\n`,
  );
  const file = join(root, "manifest.json");
  const save = () => writeFile(file, JSON.stringify(manifest));
  await save();
  return { root, build, manifest, file, save };
}

test("release review binds every binary, IDL, source revision and checksum list", async (t) => {
  const f = await fixture(t);
  await checkRelease(f.file);
  const binary = join(f.build, "akt.so");
  const original = await readFile(binary);
  await writeFile(binary, Buffer.concat([original, Buffer.from("changed")]));
  await assert.rejects(checkRelease(f.file), /artifact hash mismatch/);
  await writeFile(binary, original);
  f.manifest.programs.akt = web3.Keypair.generate().publicKey.toBase58();
  await f.save();
  await assert.rejects(checkRelease(f.file), /IDL program ID mismatch/);
  f.manifest.programs.akt = f.manifest.programs.access_manager;
  await f.save();
  await assert.rejects(checkRelease(f.file), /IDs must be distinct/);
});

test("release rejects development IDs, wrong source and unreviewed checksum changes", async (t) => {
  const f = await fixture(t);
  await writeFile(
    join(f.build, "build-info.txt"),
    `commit ${"b".repeat(40)}\n`,
  );
  await assert.rejects(checkRelease(f.file), /unreviewed build-info/);
  await writeFile(join(f.build, "SHA256SUMS"), "changed\n");
  await assert.rejects(checkRelease(f.file), /unreviewed SHA256SUMS/);
  f.manifest.programs.akt = "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5";
  await f.save();
  await assert.rejects(checkRelease(f.file), /development program IDs/);
});

test("real admin CLI refuses direct mainnet changes before loading a signer", async (t) => {
  const server = createServer(async (request, response) => {
    let body = "";
    for await (const chunk of request) body += chunk;
    const rpc = JSON.parse(body);
    assert.equal(rpc.method, "getGenesisHash");
    response.setHeader("content-type", "application/json");
    response.end(
      JSON.stringify({ jsonrpc: "2.0", id: rpc.id, result: MAINNET_GENESIS }),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  for (const command of ["execute", "set-bridge-enabled", "set-mint-policy"]) {
    await assert.rejects(
      exec(
        process.execPath,
        [
          "--import",
          "tsx",
          "local/bridge-admin.ts",
          command,
          "--genesis",
          MAINNET_GENESIS,
          "--url",
          `http://127.0.0.1:${address.port}`,
          "--keypair",
          "/nonexistent-ceremony-key",
        ],
        { cwd: repo },
      ),
      /Mainnet admin changes go through the multisig/,
    );
  }
});

test("preflight refuses a wrong cluster without a signer", async (t) => {
  const f = await fixture(t);
  const server = createServer(async (request, response) => {
    let body = "";
    for await (const chunk of request) body += chunk;
    const rpc = JSON.parse(body);
    assert.equal(rpc.method, "getGenesisHash");
    response.setHeader("content-type", "application/json");
    response.end(
      JSON.stringify({ jsonrpc: "2.0", id: rpc.id, result: "another-cluster" }),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await assert.rejects(
    exec(
      process.execPath,
      [
        "--import",
        "tsx",
        "deploy/programs.ts",
        "preflight",
        "--manifest",
        f.file,
        "--url",
        `http://127.0.0.1:${address.port}`,
      ],
      { cwd: repo, env: { ...process.env, PATH: f.root } },
    ),
    /wrong Solana cluster/,
  );
});

test("preflight checks existing programs and verification rejects wrong authority", async (t) => {
  const f = await fixture(t);
  const loader = "BPFLoaderUpgradeab1e11111111111111111111111";
  const accounts = new Map<string, unknown>();
  for (const name of names) {
    const id = new web3.PublicKey(f.manifest.programs[name]);
    const dataId = web3.PublicKey.findProgramAddressSync(
      [id.toBuffer()],
      new web3.PublicKey(loader),
    )[0];
    const program = Buffer.alloc(36);
    program.writeUInt32LE(2);
    dataId.toBuffer().copy(program, 4);
    const binary = await readFile(join(f.build, `${name}.so`));
    const data = Buffer.alloc(45 + binary.length + 32);
    data.writeUInt32LE(3);
    data[12] = 1;
    new web3.PublicKey(f.manifest.ceremonyAuthority).toBuffer().copy(data, 13);
    binary.copy(data, 45);
    const account = (bytes: Buffer, executable: boolean) => ({
      data: [bytes.toString("base64"), "base64"],
      executable,
      owner: loader,
      lamports: 100,
      rentEpoch: 0,
    });
    accounts.set(id.toBase58(), account(program, true));
    accounts.set(dataId.toBase58(), account(data, false));
  }
  const server = createServer(async (request, response) => {
    let body = "";
    for await (const chunk of request) body += chunk;
    const rpc = JSON.parse(body);
    let result;
    switch (rpc.method) {
      case "getGenesisHash":
        result = MAINNET_GENESIS;
        break;
      case "getSlot":
        result = 1;
        break;
      case "getAccountInfo":
        result = {
          context: { slot: 1 },
          value: accounts.get(rpc.params[0]) ?? null,
        };
        break;
      case "getMultipleAccounts":
        result = {
          context: { slot: 1 },
          value: rpc.params[0].map((key: string) => accounts.get(key) ?? null),
        };
        break;
      default:
        throw new Error(`unexpected RPC: ${rpc.method}`);
    }
    response.setHeader("content-type", "application/json");
    response.end(JSON.stringify({ jsonrpc: "2.0", id: rpc.id, result }));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  const args = [
    "--import",
    "tsx",
    "deploy/programs.ts",
    "preflight",
    "--manifest",
    f.file,
    "--url",
    `http://127.0.0.1:${address.port}`,
  ];
  const result = await exec(process.execPath, args, {
    cwd: repo,
    env: { ...process.env, PATH: f.root },
  });
  assert.match(result.stdout, /"missing": \[\]/);
  args[3] = "verify";
  args.push("--authority", "ceremony");
  await exec(process.execPath, args, {
    cwd: repo,
    env: { ...process.env, PATH: f.root },
  });
  const id = new web3.PublicKey(f.manifest.programs.akt);
  const dataId = web3.PublicKey.findProgramAddressSync(
    [id.toBuffer()],
    new web3.PublicKey(loader),
  )[0];
  const invalid = Buffer.alloc(46);
  invalid.writeUInt32LE(3);
  invalid[12] = 1;
  new web3.PublicKey(f.manifest.upgradeAuthority).toBuffer().copy(invalid, 13);
  accounts.set(dataId.toBase58(), {
    data: [invalid.toString("base64"), "base64"],
    executable: false,
    owner: loader,
    lamports: 100,
    rentEpoch: 0,
  });
  await assert.rejects(
    exec(process.execPath, args, {
      cwd: repo,
      env: { ...process.env, PATH: f.root },
    }),
    /wrong upgrade authority/,
  );
  accounts.delete(f.manifest.programs.access_manager);
  args[3] = "preflight";
  args.splice(-2);
  await assert.rejects(
    exec(process.execPath, args, {
      cwd: repo,
      env: { ...process.env, PATH: f.root },
    }),
    /wrong upgrade authority/,
  );
});
