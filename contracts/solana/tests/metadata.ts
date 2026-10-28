import assert from "node:assert/strict";
import type { TestContext } from "node:test";
import { Program, web3 } from "@coral-xyz/anchor";
import { getMint, TOKEN_PROGRAM_ID } from "@solana/spl-token";
import { TokenStandard } from "@metaplex-foundation/mpl-token-metadata";
import { unwrapOption } from "@metaplex-foundation/umi";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";
import {
  METADATA_PROGRAM_ID,
  metadataAddress,
  readTokenMetadata,
} from "../shared/token-metadata";
import { emittedEvent, rejectsWithAnchorCode, testProvider } from "./helpers";

export async function runMetadataTests(t: TestContext) {
  const provider = testProvider();
  const program = new Program<AktSolana>(idl, provider);
  const derive = (seed: string) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from(seed)],
      program.programId,
    )[0];
  const mint = derive("mint");
  const mintAuthority = derive("mint_authority");
  const metadata = metadataAddress(mint);
  const [programData] = web3.PublicKey.findProgramAddressSync(
    [program.programId.toBuffer()],
    new web3.PublicKey("BPFLoaderUpgradeab1e11111111111111111111111"),
  );
  const accounts = {
    admin: {
      program: program.programId,
      programData,
      authority: provider.publicKey,
    },
    config: derive("config"),
    mint,
    mintAuthority,
    metadata,
    metadataProgram: METADATA_PROGRAM_ID,
    systemProgram: web3.SystemProgram.programId,
    rent: web3.SYSVAR_RENT_PUBKEY,
  };
  const uri = "https://metadata.example.test/akt.json";
  const attacker = web3.Keypair.generate();
  const before = await getMint(provider.connection, mint);

  await t.test("rejects unauthorized metadata creation", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .setTokenMetadata(uri)
          .accountsStrict({
            ...accounts,
            admin: { ...accounts.admin, authority: attacker.publicKey },
          })
          .signers([attacker])
          .rpc(),
      "InvalidAuthority",
    );
    assert.equal(
      await readTokenMetadata(provider.connection, mint, "confirmed"),
      null,
    );
  });
  await t.test("rejects substituted metadata account and program", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .setTokenMetadata(uri)
          .accountsStrict({ ...accounts, metadata: attacker.publicKey })
          .rpc(),
      "ConstraintSeeds",
    );
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .setTokenMetadata(uri)
          .accountsStrict({ ...accounts, metadataProgram: TOKEN_PROGRAM_ID })
          .rpc(),
      "InvalidProgramId",
    );
  });
  await t.test("rejects invalid metadata URIs", async () => {
    for (const invalid of [
      "",
      "http://example.test/a.json",
      "https://",
      "https://example.test/a\n",
      `https://example.test/${"a".repeat(200)}`,
    ]) {
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .setTokenMetadata(invalid)
            .accountsStrict(accounts)
            .rpc(),
        "InvalidMetadataUri",
      );
    }
  });
  await t.test(
    "creates standard fungible metadata despite PDA prefunding",
    async () => {
      await provider.sendAndConfirm(
        new web3.Transaction().add(
          web3.SystemProgram.transfer({
            fromPubkey: provider.publicKey,
            toPubkey: metadata,
            lamports:
              await provider.connection.getMinimumBalanceForRentExemption(0),
          }),
        ),
      );
      const set = await emittedEvent(
        program,
        await program.methods
          .setTokenMetadata(uri)
          .accountsStrict(accounts)
          .rpc(),
        "tokenMetadataSet",
      );
      assert.ok(set.mint.equals(mint));
      assert.ok(set.metadata.equals(metadata));
      assert.equal(set.uri, uri);
      const data = await readTokenMetadata(
        provider.connection,
        mint,
        "confirmed",
      );
      assert.ok(data);
      assert.equal(data.name, "Akash Network");
      assert.equal(data.symbol, "AKT");
      assert.equal(data.uri, uri);
      assert.equal(data.updateAuthority, mintAuthority.toBase58());
      assert.equal(data.sellerFeeBasisPoints, 0);
      assert.equal(data.isMutable, true);
      assert.equal(unwrapOption(data.tokenStandard), TokenStandard.Fungible);
      assert.deepEqual(await getMint(provider.connection, mint), before);
    },
  );
  await t.test("rejects unauthorized metadata updates", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .setTokenMetadata("https://attacker.test/akt.json")
          .accountsStrict({
            ...accounts,
            admin: { ...accounts.admin, authority: attacker.publicKey },
          })
          .signers([attacker])
          .rpc(),
      "InvalidAuthority",
    );
    assert.equal(
      (await readTokenMetadata(provider.connection, mint, "confirmed"))?.uri,
      uri,
    );
  });
  await t.test(
    "updates and repeats metadata without changing the mint",
    async () => {
      const next = "https://metadata.example.test/akt-v2.json";
      for (let i = 0; i < 2; i++)
        await program.methods
          .setTokenMetadata(next)
          .accountsStrict(accounts)
          .rpc();
      const data = await readTokenMetadata(
        provider.connection,
        mint,
        "confirmed",
      );
      assert.equal(data?.uri, next);
      assert.equal(data?.updateAuthority, mintAuthority.toBase58());
      assert.deepEqual(await getMint(provider.connection, mint), before);
    },
  );
}
