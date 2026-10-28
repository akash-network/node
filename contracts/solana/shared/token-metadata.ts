import assert from "node:assert/strict";
import { web3 } from "@coral-xyz/anchor";
import {
  getMetadataAccountDataSerializer,
  Key,
  MPL_TOKEN_METADATA_PROGRAM_ID,
} from "@metaplex-foundation/mpl-token-metadata";

export const METADATA_PROGRAM_ID = new web3.PublicKey(
  MPL_TOKEN_METADATA_PROGRAM_ID,
);

export function metadataAddress(mint: web3.PublicKey) {
  return web3.PublicKey.findProgramAddressSync(
    [Buffer.from("metadata"), METADATA_PROGRAM_ID.toBuffer(), mint.toBuffer()],
    METADATA_PROGRAM_ID,
  )[0];
}

export async function readTokenMetadata(
  connection: web3.Connection,
  mint: web3.PublicKey,
  commitment: web3.Commitment = "finalized",
) {
  const account = await connection.getAccountInfo(
    metadataAddress(mint),
    commitment,
  );
  if (!account) return null;
  assert.ok(account.owner.equals(METADATA_PROGRAM_ID), "wrong metadata owner");
  const [data] = getMetadataAccountDataSerializer().deserialize(account.data);
  assert.equal(data.key, Key.MetadataV1);
  assert.equal(data.mint, mint.toBase58());
  return data;
}
