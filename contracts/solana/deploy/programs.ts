import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile, realpath } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { web3 } from "@coral-xyz/anchor";
import { z } from "zod";
import { MAINNET_GENESIS } from "../shared/networks";

const names = ["access_manager", "ics07_tendermint", "akt_solana"] as const;
const developmentIds = new Set([
  "4fMih2CidrXPeRx77kj3QcuBZwREYtxEbXjURUgadoe1",
  "HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD",
  "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
]);
const publicKey = z.string().refine((value) => {
  try {
    return new web3.PublicKey(value).toBase58() === value;
  } catch {
    return false;
  }
}, "expected a canonical Solana public key");
const manifestSchema = z.strictObject({
  version: z.literal(1),
  sourceCommit: z.string().regex(/^[0-9a-f]{40}$/),
  buildDirectory: z.string().min(1),
  buildInfoSha256: z.string().regex(/^[0-9a-f]{64}$/),
  checksumsSha256: z.string().regex(/^[0-9a-f]{64}$/),
  solanaGenesisHash: z.literal(MAINNET_GENESIS),
  ceremonyAuthority: publicKey,
  upgradeAuthority: publicKey,
  programs: z.strictObject({
    access_manager: publicKey,
    ics07_tendermint: publicKey,
    akt_solana: publicKey,
  }),
});
const digest = (data: Buffer) =>
  createHash("sha256").update(data).digest("hex");

export async function checkRelease(path: string) {
  const manifest = manifestSchema.parse(
    JSON.parse(await readFile(path, "utf8")),
  );
  const directory = await realpath(
    resolve(dirname(path), manifest.buildDirectory),
  );
  assert.notEqual(
    manifest.ceremonyAuthority,
    manifest.upgradeAuthority,
    "ceremony and multisig authorities must differ",
  );
  const ids = names.map((name) => manifest.programs[name]);
  assert.equal(new Set(ids).size, names.length, "program IDs must be distinct");
  for (const id of ids) {
    assert.ok(
      !developmentIds.has(id),
      "production cannot use development program IDs",
    );
    assert.ok(
      ![manifest.ceremonyAuthority, manifest.upgradeAuthority].includes(id),
      "program IDs must differ from authority addresses",
    );
  }
  const sums = await readFile(join(directory, "SHA256SUMS"));
  assert.equal(digest(sums), manifest.checksumsSha256, "unreviewed SHA256SUMS");
  const entries = new Map<string, string>();
  for (const line of sums.toString("utf8").trim().split("\n")) {
    const match = /^([0-9a-f]{64})  ([a-z0-9_]+\.(?:so|json|ts))$/.exec(line);
    assert.ok(match, "invalid SHA256SUMS entry");
    assert.ok(!entries.has(match[2]), "duplicate SHA256SUMS entry");
    entries.set(match[2], match[1]);
  }
  assert.equal(
    entries.size,
    names.length * 3,
    "expected three programs, IDLs and TypeScript interfaces",
  );
  for (const name of names) {
    for (const suffix of ["so", "json", "ts"]) {
      const file = `${name}.${suffix}`;
      const path = join(directory, file);
      assert.equal(
        await realpath(path),
        path,
        "release artifacts must not be symlinks",
      );
      const bytes = await readFile(path);
      assert.equal(
        digest(bytes),
        entries.get(file),
        `artifact hash mismatch: ${file}`,
      );
      if (suffix === "so") {
        assert.ok(
          bytes.length > 4 &&
            bytes.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46])),
          `not an ELF program: ${file}`,
        );
      } else if (suffix === "json") {
        const idl = z
          .object({ address: publicKey })
          .parse(JSON.parse(bytes.toString("utf8")));
        assert.equal(
          idl.address,
          manifest.programs[name],
          `IDL program ID mismatch: ${name}`,
        );
      }
    }
  }
  const info = await readFile(join(directory, "build-info.txt"));
  assert.equal(
    digest(info),
    manifest.buildInfoSha256,
    "unreviewed build-info.txt",
  );
  assert.equal(
    info.toString("utf8").split("\n")[0],
    `commit ${manifest.sourceCommit}`,
    "build source mismatch",
  );
  return { manifest, directory };
}

export async function checkCluster(
  connection: web3.Connection,
  expected: string,
) {
  assert.equal(
    await connection.getGenesisHash(),
    expected,
    "wrong Solana cluster",
  );
  // An RPC failure must never be interpreted as an absent program.
  await connection.getSlot("finalized");
}

// Upgradeable loader-v3 state, also used by bridge-admin's program checks.
// Fetch both accounts at one finalized context and validate their linkage.
export async function verifyProgram(
  connection: web3.Connection,
  id: web3.PublicKey,
  binary: Buffer,
  authority: string,
) {
  const loader = new web3.PublicKey(
    "BPFLoaderUpgradeab1e11111111111111111111111",
  );
  const dataAddress = web3.PublicKey.findProgramAddressSync(
    [id.toBuffer()],
    loader,
  )[0];
  const [program, data] = await connection.getMultipleAccountsInfo(
    [id, dataAddress],
    "finalized",
  );
  assert.ok(
    program?.executable && program.owner.equals(loader),
    "not an upgradeable program",
  );
  assert.equal(program.data.length, 4 + 32, "invalid loader Program state");
  assert.equal(program.data.readUInt32LE(), 2, "invalid loader Program tag");
  assert.ok(
    new web3.PublicKey(program.data.subarray(4)).equals(dataAddress),
    "wrong ProgramData link",
  );
  assert.ok(
    data && !data.executable && data.owner.equals(loader),
    "invalid ProgramData account",
  );
  assert.ok(data.data.length >= 4 + 8 + 1 + 32, "truncated ProgramData");
  let remaining = data.data;
  const take = (size: number) => {
    const bytes = remaining.subarray(0, size);
    remaining = remaining.subarray(size);
    return bytes;
  };
  assert.equal(take(4).readUInt32LE(), 3, "invalid ProgramData tag");
  take(8); // deployment slot
  assert.equal(take(1)[0], 1, "program has no upgrade authority");
  assert.equal(
    new web3.PublicKey(take(32)).toBase58(),
    authority,
    "wrong upgrade authority",
  );
  assert.ok(
    remaining.length >= binary.length && take(binary.length).equals(binary),
    "deployed bytes differ from reviewed artifact",
  );
  assert.ok(
    remaining.every((byte) => byte === 0),
    "nonzero program padding",
  );
}

async function main() {
  const { values, positionals } = parseArgs({
    allowPositionals: true,
    options: {
      manifest: { type: "string" },
      url: { type: "string" },
      authority: { type: "string" },
      help: { type: "boolean" },
    },
  });
  if (values.help) {
    process.stdout.write(
      "programs.ts check|preflight|verify --manifest FILE [--url RPC]\n" +
        "verify: --authority ceremony|multisig\n",
    );
    return;
  }
  const command = z
    .enum(["check", "preflight", "verify"])
    .parse(positionals[0]);
  assert.equal(positionals.length, 1, "expected one command");
  assert.ok(values.manifest, "--manifest is required");
  assert.ok(
    command === "verify" || !values.authority,
    "--authority is only for verify",
  );
  const { manifest, directory } = await checkRelease(resolve(values.manifest));
  if (command === "check") {
    process.stdout.write(`Release checked: ${manifest.sourceCommit}\n`);
    return;
  }
  assert.ok(values.url, "--url is required; no default cluster is used");
  const url = new URL(values.url);
  assert.ok(
    ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password,
    "use an HTTP(S) RPC or local authenticated proxy",
  );
  const connection = new web3.Connection(url.href, {
    commitment: "finalized",
    disableRetryOnRateLimit: true,
    fetch: (input, init) =>
      fetch(input, { ...init, signal: AbortSignal.timeout(30_000) }),
  });
  await checkCluster(connection, manifest.solanaGenesisHash);
  const verify = async (name: (typeof names)[number], authority: string) => {
    await verifyProgram(
      connection,
      new web3.PublicKey(manifest.programs[name]),
      await readFile(join(directory, `${name}.so`)),
      authority,
    );
    process.stdout.write(`${name}: finalized bytes and authority verified\n`);
  };
  if (command === "verify") {
    const phase = z.enum(["ceremony", "multisig"]).parse(values.authority);
    for (const name of names) {
      await verify(
        name,
        phase === "ceremony"
          ? manifest.ceremonyAuthority
          : manifest.upgradeAuthority,
      );
    }
    return;
  }
  const missing: (typeof names)[number][] = [];
  // Preflight verifies occupied addresses before the operator deploys missing programs.
  for (const name of names) {
    const account = await connection.getAccountInfo(
      new web3.PublicKey(manifest.programs[name]),
      "finalized",
    );
    if (account === null) missing.push(name);
    else {
      assert.ok(
        account.executable,
        `${name} address is occupied by a non-program account`,
      );
      await verify(name, manifest.ceremonyAuthority);
    }
  }
  process.stdout.write(
    JSON.stringify({ sourceCommit: manifest.sourceCommit, missing }, null, 2) +
      "\n",
  );
}

if (require.main === module) {
  void main().catch((error: unknown) => {
    process.stderr.write(
      `${error instanceof Error ? error.message : "release verification failed"}\n`,
    );
    process.exitCode = 1;
  });
}
