import assert from "node:assert/strict";
import { readdir } from "node:fs/promises";
import { join } from "node:path";
import { AnchorProvider, web3 } from "@coral-xyz/anchor";
import { object } from "./common";
import { optionalJson, atomicJson, withUploadLock } from "./durable";
import {
  resumeTransaction,
  sendTransaction,
  type TransactionResult,
} from "./transactions";

export async function reclaimLookups(
  wallet: AnchorProvider,
  directory: string,
) {
  return withUploadLock(join(directory, "payer.lock"), () =>
    reclaimLocked(wallet, directory),
  );
}
async function reclaimLocked(wallet: AnchorProvider, directory: string) {
  let names: string[];
  try {
    names = await readdir(directory);
  } catch (error) {
    if (error instanceof Error && "code" in error && error.code === "ENOENT")
      return [];
    throw error;
  }
  const closed: string[] = [];
  const genesis = await wallet.connection.getGenesisHash();
  for (const name of names.filter((n) => /^[0-9a-f]{64}\.json$/.test(n))) {
    if (closed.length >= 1) break; // Bound maintenance latency per relayer tick.
    const path = join(directory, name);
    let saved = object(await optionalJson(path));
    assert.equal(saved.genesis, genesis);
    assert.equal(saved.payer, wallet.publicKey.toBase58());
    if (saved.lookup === undefined) continue;
    assert.ok(typeof saved.lookup === "string");
    const address = new web3.PublicKey(saved.lookup);
    const pendingPath = join(
      directory,
      name.replace(".json", ".close.pending.json"),
    );
    async function observe(result: TransactionResult) {
      if (result.error !== null) {
        // A finalized rejection is consumed, not retried forever as an unknown
        // outcome. Re-read the table below before constructing another close.
        saved = { ...saved, lookupCloseFailure: result };
        await atomicJson(path, saved);
        return;
      }
      assert.ok(typeof saved.lookupRentClosing === "number");
      saved = {
        ...saved,
        lookupClosed: true,
        lookupRentReturned: saved.lookupRentClosing,
        lookupCloseTransaction: result.signature,
      };
      await atomicJson(path, saved);
      closed.push(address.toBase58());
      console.log(
        JSON.stringify({
          event: "lookup-rent-returned",
          address: address.toBase58(),
          lamports: saved.lookupRentClosing,
          signature: result.signature,
        }),
      );
    }
    await resumeTransaction(wallet, pendingPath, observe);
    if (saved.lookupClosed === true) continue;
    const table = (
      await wallet.connection.getAddressLookupTable(address, {
        commitment: "finalized",
      })
    ).value;
    if (!table) {
      saved = { ...saved, lookupClosed: true, lookupClosureUnobserved: true };
      await atomicJson(path, saved);
      continue;
    }
    assert.ok(table.state.authority?.equals(wallet.publicKey));
    if (table.state.deactivationSlot === 18446744073709551615n) continue;
    const slots = await wallet.connection.getAccountInfo(
      web3.SYSVAR_SLOT_HASHES_PUBKEY,
    );
    assert.ok(slots && slots.data.length >= 8);
    const count = Number(slots.data.readBigUInt64LE());
    assert.ok(count > 0 && count <= 512 && slots.data.length >= 8 + count * 40);
    const oldest = slots.data.readBigUInt64LE(8 + (count - 1) * 40);
    if (oldest <= table.state.deactivationSlot) continue;
    const before = await wallet.connection.getAccountInfo(address);
    assert.ok(before);
    saved = { ...saved, lookupRentClosing: before.lamports };
    await atomicJson(path, saved);
    const result = await sendTransaction({
      wallet,
      stage: "lookup-close",
      pendingPath,
      observe,
      instructions: [
        web3.AddressLookupTableProgram.closeLookupTable({
          lookupTable: address,
          authority: wallet.publicKey,
          recipient: wallet.publicKey,
        }),
      ],
    });
    if (result.error === null)
      assert.equal(
        await wallet.connection.getAccountInfo(address, "finalized"),
        null,
      );
  }
  return closed;
}
