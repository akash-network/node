import assert from "node:assert/strict";
import { constants, open } from "node:fs/promises";
import { DirectSecp256k1Wallet } from "@cosmjs/proto-signing";

export async function loadSigner(path: string, expected?: string) {
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const stat = await file.stat();
    assert.ok(
      stat.isFile() &&
        (stat.mode & 0o077) === 0 &&
        stat.uid === process.getuid?.(),
      "signing key must be an owner-only regular file",
    );
    const raw = (await file.readFile("utf8")).trim();
    assert.ok(
      /^[0-9a-f]{64}$/.test(raw),
      "key file must contain a 32-byte lowercase hex secp256k1 key",
    );
    const scalar = BigInt(`0x${raw}`);
    assert.ok(
      scalar > 0n &&
        scalar <
          0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n,
      "invalid signing key scalar",
    );
    const key = Buffer.from(raw, "hex");
    const wallet = await DirectSecp256k1Wallet.fromKey(key, "akash");
    // The wallet retains the key for signing; never write it to output.
    if (expected) {
      assert.equal(
        (await wallet.getAccounts())[0]?.address,
        expected,
        "signer does not match reporter",
      );
    }
    return wallet;
  } finally {
    await file.close();
  }
}
