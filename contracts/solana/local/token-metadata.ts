import assert from "node:assert/strict";
import { parseArgs } from "node:util";
import { Program, web3 } from "@coral-xyz/anchor";
import { getMint } from "@solana/spl-token";
import { TokenStandard } from "@metaplex-foundation/mpl-token-metadata";
import { unwrapOption } from "@metaplex-foundation/umi";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";
import {
  METADATA_PROGRAM_ID,
  metadataAddress,
  readTokenMetadata,
} from "../shared/token-metadata";
import { object, pda, programData, PROGRAM_ID, provider } from "./common";

function httpsUrl(value: string) {
  const url = new URL(value);
  assert.equal(url.protocol, "https:");
  assert.ok(
    !url.username && !url.password && !url.hash,
    "plain HTTPS URL required",
  );
  return url;
}

async function main() {
  const { values } = parseArgs({
    options: {
      uri: { type: "string" },
      genesis: { type: "string" },
    },
  });
  assert.ok(
    values.uri && values.genesis,
    "Usage: npm run local:metadata -- --uri https://HOST/token/akt.json --genesis HASH",
  );
  assert.ok(
    /^[\x21-\x7e]{9,200}$/.test(values.uri),
    "URI must fit Metaplex's 200-byte field",
  );
  const uri = httpsUrl(values.uri).href;
  assert.equal(uri, values.uri, "use the canonical URL spelling");
  const wallet = await provider();
  const genesis = await wallet.connection.getGenesisHash();
  assert.equal(genesis, values.genesis, "wrong Solana ledger");
  assert.notEqual(
    genesis,
    "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp",
    "use a reviewed multisig transaction for mainnet",
  );
  const metaplex = await wallet.connection.getAccountInfo(
    METADATA_PROGRAM_ID,
    "finalized",
  );
  assert.ok(
    metaplex?.executable,
    "Metaplex is missing. Existing ledgers cannot load genesis programs on restart; see docs/token-metadata.md.",
  );

  const response = await fetch(uri, {
    signal: AbortSignal.timeout(15_000),
    redirect: "error",
  });
  assert.ok(response.ok, "metadata JSON is not reachable");
  assert.match(
    response.headers.get("content-type") ?? "",
    /^application\/json\b/,
  );
  const json = object(await response.json());
  assert.equal(json.name, "Akash Network");
  assert.equal(json.symbol, "AKT");
  assert.ok(typeof json.image === "string", "image URL required");
  const image = await fetch(httpsUrl(json.image), {
    signal: AbortSignal.timeout(15_000),
    redirect: "error",
  });
  assert.ok(image.ok, "token logo is not reachable");
  assert.match(image.headers.get("content-type") ?? "", /^image\/png\b/);
  const logo = Buffer.from(await image.arrayBuffer());
  assert.ok(
    logo.length <= 1_000_000 &&
      logo.subarray(0, 8).equals(Buffer.from("89504e470d0a1a0a", "hex")),
    "PNG logo required, at most 1 MB",
  );

  const program = new Program<AktSolana>(idl, wallet);
  const mint = pda("mint");
  const mintAuthority = pda("mint_authority");
  const before = await getMint(wallet.connection, mint, "finalized");
  const old = await readTokenMetadata(wallet.connection, mint);
  let signature: string | null = null;
  if (
    !old ||
    old.uri !== uri ||
    old.name !== "Akash Network" ||
    old.symbol !== "AKT"
  ) {
    signature = await program.methods
      .setTokenMetadata(uri)
      .accountsStrict({
        admin: {
          program: PROGRAM_ID,
          programData: programData(PROGRAM_ID),
          authority: wallet.publicKey,
        },
        config: pda("config"),
        mint,
        mintAuthority,
        metadata: metadataAddress(mint),
        metadataProgram: METADATA_PROGRAM_ID,
        systemProgram: web3.SystemProgram.programId,
        rent: web3.SYSVAR_RENT_PUBKEY,
      })
      .rpc({ commitment: "finalized", preflightCommitment: "finalized" });
  }
  const metadata = await readTokenMetadata(wallet.connection, mint);
  assert.ok(metadata);
  assert.equal(metadata.name, "Akash Network");
  assert.equal(metadata.symbol, "AKT");
  assert.equal(metadata.uri, uri);
  assert.equal(metadata.updateAuthority, mintAuthority.toBase58());
  assert.equal(metadata.sellerFeeBasisPoints, 0);
  assert.equal(unwrapOption(metadata.tokenStandard), TokenStandard.Fungible);
  const after = await getMint(wallet.connection, mint, "finalized");
  assert.ok(after.mintAuthority?.equals(mintAuthority));
  assert.equal(after.decimals, 6);
  assert.equal(after.freezeAuthority, null);
  // Other users may mint while this operator runs. Metadata cannot change mint
  // supply: the instruction passes the mint read-only, and never invokes SPL.
  console.log(
    JSON.stringify(
      {
        genesis,
        mint: mint.toBase58(),
        metadata: metadataAddress(mint).toBase58(),
        name: metadata.name,
        symbol: metadata.symbol,
        uri,
        updateAuthority: metadata.updateAuthority,
        signature,
        supplyBefore: before.supply.toString(),
        supplyAfter: after.supply.toString(),
      },
      null,
      2,
    ),
  );
}

main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
