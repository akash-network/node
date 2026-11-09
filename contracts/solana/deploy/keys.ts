import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { open } from "node:fs/promises";
import { isAbsolute } from "node:path";
import { web3 } from "@coral-xyz/anchor";
import { DirectSecp256k1Wallet } from "@cosmjs/proto-signing";
import { loadPayer } from "../local/relay-config";
import { loadSigner } from "../reporter/key";

export async function keyAddress(role: "relayer" | "reporter", path: string) {
  assert.ok(isAbsolute(path), "key path must be absolute");
  if (role === "relayer") return (await loadPayer(path)).publicKey.toBase58();
  const wallet = await loadSigner(path);
  const [account] = await wallet.getAccounts();
  return account.address;
}

export async function createKey(role: "relayer" | "reporter", path: string) {
  assert.ok(isAbsolute(path), "key path must be absolute");
  let contents: string;
  let address: string;
  if (role === "relayer") {
    const key = web3.Keypair.generate();
    contents = JSON.stringify(Array.from(key.secretKey));
    address = key.publicKey.toBase58();
  } else {
    const key = randomBytes(32);
    const wallet = await DirectSecp256k1Wallet.fromKey(key, "akash");
    const [account] = await wallet.getAccounts();
    contents = key.toString("hex");
    address = account.address;
  }
  // Exclusive creation also refuses existing symlinks and never replaces a key.
  const file = await open(path, "wx", 0o600);
  try {
    await file.writeFile(`${contents}\n`);
    await file.sync();
  } finally {
    await file.close();
  }
  return address;
}
