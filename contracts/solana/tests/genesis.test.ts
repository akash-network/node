import assert from "node:assert/strict";
import test from "node:test";
import { Program, web3 } from "@coral-xyz/anchor";
import {
  emittedEvent,
  rejectsWithAnchorCode,
  rejectsWithProgramLogs,
  testProvider,
} from "./helpers";
import { runBridgeTests } from "./bridge";
import { runMetadataTests } from "./metadata";
import { runSinkProofTests } from "./sink-proof";
import {
  AuthorityType,
  TOKEN_PROGRAM_ID,
  createAssociatedTokenAccountIdempotentInstruction,
  createMintToInstruction,
  createSetAuthorityInstruction,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";

const UPGRADEABLE_LOADER = new web3.PublicKey(
  "BPFLoaderUpgradeab1e11111111111111111111111",
);

test("AKT issuance", { timeout: 180_000 }, async (t) => {
  const provider = testProvider();
  const program = new Program<AktSolana>(idl, provider);
  const connection = provider.connection;
  const attacker = web3.Keypair.generate();

  const [config, configBump] = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("config")],
    program.programId,
  );
  const [mint] = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("mint")],
    program.programId,
  );
  const [mintAuthority, authorityBump] = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("mint_authority")],
    program.programId,
  );
  const [programData] = web3.PublicKey.findProgramAddressSync(
    [program.programId.toBuffer()],
    UPGRADEABLE_LOADER,
  );
  const accounts = {
    program: program.programId,
    programData,
    authority: provider.publicKey,
    config,
    mint,
    mintAuthority,
    tokenProgram: TOKEN_PROGRAM_ID,
    systemProgram: web3.SystemProgram.programId,
  };

  assert.equal(
    await connection.getAccountInfo(config, "confirmed"),
    null,
    "genesis tests require a fresh local validator",
  );
  await provider.sendAndConfirm(
    new web3.Transaction().add(
      web3.SystemProgram.transfer({
        fromPubkey: provider.publicKey,
        toPubkey: attacker.publicKey,
        lamports: web3.LAMPORTS_PER_SOL,
      }),
    ),
  );

  await t.test("rejects an unauthorized first initializer", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .initialize()
          .accountsStrict({ ...accounts, authority: attacker.publicKey })
          .signers([attacker])
          .rpc(),
      "UnauthorizedInitializer",
    );
    assert.equal(await connection.getAccountInfo(config, "confirmed"), null);
    assert.equal(await connection.getAccountInfo(mint, "confirmed"), null);
  });

  await t.test("rejects a substituted program identity", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .initialize()
          .accountsStrict({ ...accounts, program: TOKEN_PROGRAM_ID })
          .rpc(),
      "InvalidProgramId",
    );
  });

  await t.test("rejects a noncanonical mint address", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .initialize()
          .accountsStrict({ ...accounts, mint: attacker.publicKey })
          .rpc(),
      "ConstraintSeeds",
    );
  });

  await t.test("rejects a substituted mint authority", async () => {
    await rejectsWithAnchorCode(
      () =>
        program.methods
          .initialize()
          .accountsStrict({ ...accounts, mintAuthority: attacker.publicKey })
          .rpc(),
      "ConstraintSeeds",
    );
  });

  await t.test(
    "creates the mint with zero supply and disables bridge issuance",
    async () => {
      // A third party prefunding deterministic addresses must not block launch.
      // New accounts must have the minimum rent-exempt balance, even with no data.
      const prefundLamports =
        await connection.getMinimumBalanceForRentExemption(0);
      await provider.sendAndConfirm(
        new web3.Transaction().add(
          ...[config, mint].map((toPubkey) =>
            web3.SystemProgram.transfer({
              fromPubkey: provider.publicKey,
              toPubkey,
              lamports: prefundLamports,
            }),
          ),
        ),
      );
      const initialized = await emittedEvent(
        program,
        await program.methods.initialize().accountsStrict(accounts).rpc(),
        "launchInitialized",
      );
      assert.ok(initialized.mint.equals(mint));
      assert.ok(initialized.initializer.equals(provider.publicKey));

      const mintState = await getMint(connection, mint, "confirmed");
      assert.equal(mintState.decimals, 6);
      assert.equal(mintState.supply, 0n);
      assert.ok(mintState.mintAuthority?.equals(mintAuthority));
      assert.equal(mintState.freezeAuthority, null);

      // Genesis takes no treasury and creates no token account.
      assert.deepEqual(
        idl.instructions
          .find((instruction) => instruction.name === "initialize")
          ?.accounts.map((account) => account.name),
        [
          "program",
          "program_data",
          "authority",
          "config",
          "mint",
          "mint_authority",
          "token_program",
          "system_program",
        ],
      );
      const tokenAccounts = await connection.getProgramAccounts(
        TOKEN_PROGRAM_ID,
        {
          commitment: "confirmed",
          filters: [
            { dataSize: 165 },
            { memcmp: { offset: 0, bytes: mint.toBase58() } },
          ],
        },
      );
      assert.equal(tokenAccounts.length, 0);

      const launch = await program.account.launchConfig.fetch(
        config,
        "confirmed",
      );
      assert.equal(launch.version, 2);
      assert.ok(launch.initializer.equals(provider.publicKey));
      assert.ok(launch.mint.equals(mint));
      assert.equal("treasury" in launch, false);
      assert.equal(launch.bridgeEnabled, false);
      assert.equal(launch.bump, configBump);
      assert.equal(launch.mintAuthorityBump, authorityBump);
      assert.ok(launch.initializedAtSlot.gtn(0));
      assert.equal(
        (await connection.getAccountInfo(config, "confirmed"))?.data.length,
        84,
      );
    },
  );

  await t.test("token metadata", runMetadataTests);

  await t.test("rejects repeating genesis", async () => {
    await rejectsWithProgramLogs(
      () =>
        program.methods
          .initialize()
          .accountsStrict(accounts)
          .preInstructions([
            web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 250_000 }),
          ])
          .rpc(),
      /already in use/,
    );
    assert.equal((await getMint(connection, mint)).supply, 0n);
  });

  await t.test(
    "rejects repeating genesis from a non-upgrade-authority signer",
    async () => {
      // Account creation runs before the authority constraint, so the
      // existing config rejects the call first.
      await rejectsWithProgramLogs(
        () =>
          program.methods
            .initialize()
            .accountsStrict({ ...accounts, authority: attacker.publicKey })
            .signers([attacker])
            .rpc(),
        /already in use/,
      );
      assert.equal((await getMint(connection, mint)).supply, 0n);
      assert.ok(
        (await program.account.launchConfig.fetch(config)).initializer.equals(
          provider.publicKey,
        ),
      );
    },
  );

  await t.test("rejects direct minting by another signer", async () => {
    const attackerTokenAccount = getAssociatedTokenAddressSync(
      mint,
      attacker.publicKey,
    );
    await provider.sendAndConfirm(
      new web3.Transaction().add(
        createAssociatedTokenAccountIdempotentInstruction(
          provider.publicKey,
          attackerTokenAccount,
          attacker.publicKey,
          mint,
        ),
      ),
    );
    await rejectsWithProgramLogs(
      () =>
        provider.sendAndConfirm(
          new web3.Transaction().add(
            createMintToInstruction(
              mint,
              attackerTokenAccount,
              attacker.publicKey,
              1n,
            ),
          ),
          [attacker],
        ),
      /owner does not match/,
    );
    assert.equal((await getMint(connection, mint)).supply, 0n);
  });

  await t.test("rejects taking over the mint authority", async () => {
    await rejectsWithProgramLogs(
      () =>
        provider.sendAndConfirm(
          new web3.Transaction().add(
            createSetAuthorityInstruction(
              mint,
              attacker.publicKey,
              AuthorityType.MintTokens,
              attacker.publicKey,
            ),
          ),
          [attacker],
        ),
      /owner does not match/,
    );
    assert.ok(
      (await getMint(connection, mint)).mintAuthority?.equals(mintAuthority),
    );
    assert.equal(
      (await program.account.launchConfig.fetch(config)).bridgeEnabled,
      false,
    );
  });
  if (process.env.SINK_PROOF_FIXTURE) {
    const fixture = process.env.SINK_PROOF_FIXTURE;
    await t.test("real Akash sink proof", (test) =>
      runSinkProofTests(test, fixture),
    );
  } else {
    await t.test("proof-gated bridge", runBridgeTests);
  }
});
