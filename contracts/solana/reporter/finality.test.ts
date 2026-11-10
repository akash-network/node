import { testTransfer } from "../tests/sink-fixtures";
import { anchorId } from "../shared/proof";
import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";
import { BN, Program, web3, type IdlAccounts } from "@coral-xyz/anchor";
import { MintLayout, TOKEN_PROGRAM_ID } from "@solana/spl-token";
import idl from "../../../.cache/solana/target/idl/akt.json";
import type { Akt } from "../../../.cache/solana/target/types/akt";
import { SolanaObserver, SolanaUnavailableError, decodeRecord } from "./solana";

async function fixture(t: TestContext) {
  const rpc = new web3.Connection("http://127.0.0.1:8899");
  const program = new Program<Akt>(idl, { connection: rpc });
  const pda = (seed: string) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from(seed)],
      program.programId,
    )[0];
  const config = {
    genesisHash: web3.Keypair.generate().publicKey.toBase58(),
    programId: program.programId,
    mint: pda("mint"),
    lightClientProgram: web3.Keypair.generate().publicKey,
    akashChainId: "akash-test",
    maxAgeSeconds: 60,
  };
  const record = decodeRecord(testTransfer());
  const status: IdlAccounts<Akt>["transferStatus"] = {
    id: anchorId(record.id),
    proofHeight: new BN(3),
    resolvedAtSlot: new BN(100),
    outcome: { minted: { amount: new BN(60), recipient: record.recipient } },
  };
  const info = (
    data: Buffer,
    owner = program.programId,
  ): web3.AccountInfo<Buffer> => ({
    data,
    owner,
    executable: false,
    lamports: 1,
    rentEpoch: 0,
  });
  const mint = Buffer.alloc(MintLayout.span);
  MintLayout.encode(
    {
      mintAuthorityOption: 1,
      mintAuthority: pda("mint_authority"),
      supply: 60n,
      decimals: 6,
      isInitialized: true,
      freezeAuthorityOption: 0,
      freezeAuthority: web3.PublicKey.default,
    },
    mint,
  );
  const accounts: (web3.AccountInfo<Buffer> | null)[] = [
    { ...info(Buffer.alloc(0)), executable: true },
    info(
      await program.coder.accounts.encode("launchConfig", {
        version: 2,
        initializer: record.recipient,
        mint: config.mint,
        bridgeEnabled: true,
        initializedAtSlot: new BN(1),
        bump: 1,
        mintAuthorityBump: 1,
      }),
    ),
    info(
      await program.coder.accounts.encode("bridgeConfig", {
        lightClientProgram: config.lightClientProgram,
        chainId: config.akashChainId,
        policy: {
          perTransferMax: new BN(100),
          dailyMintCap: new BN(1000),
          vaults: [],
        },
        mintDay: new BN(1),
        mintedToday: new BN(60),
      }),
    ),
    info(mint, TOKEN_PROGRAM_ID),
    info(await program.coder.accounts.encode("transferStatus", status)),
  ];
  t.mock.method(rpc, "getGenesisHash", async () => config.genesisHash);
  t.mock.method(rpc, "getMultipleAccountsInfoAndContext", async () => ({
    context: { slot: 110 },
    value: accounts,
  }));
  t.mock.method(rpc, "getBlockTime", async () => Math.floor(Date.now() / 1000));
  const blocks = t.mock.method(rpc, "getBlock", async () => null);
  return {
    rpc,
    record,
    accounts,
    blocks,
    observer: new SolanaObserver(rpc, config),
  };
}

test("reporter requires block evidence even when finalized status survives pruning", async (t) => {
  const f = await fixture(t);
  const result = await f.observer.observeFinalized(f.record);
  assert.equal(result.kind, "resolved");
  if (result.kind === "resolved") {
    assert.equal(result.slot, 100n);
    assert.equal(result.proofHeight, 3n);
  }
  assert.equal(f.blocks.mock.callCount(), 0);
  await assert.rejects(
    f.observer.observe(f.record),
    /resolution evidence unavailable/,
  );
  assert.equal(f.blocks.mock.callCount(), 1);
  assert.equal(f.blocks.mock.calls[0].arguments[0], 100);
  assert.deepEqual(await f.observer.deployment(), {
    contextSlot: 110n,
    bridgeEnabled: true,
  });
});

test("unavailable Solana transport is distinct from invalid state and absence", async (t) => {
  const unavailable = await fixture(t);
  t.mock.method(unavailable.rpc, "getGenesisHash", async () => {
    throw new Error("connection refused");
  });
  await assert.rejects(
    unavailable.observer.observeFinalized(unavailable.record),
    SolanaUnavailableError,
  );
  const invalid = await fixture(t);
  assert.ok(invalid.accounts[4]);
  invalid.accounts[4].owner = web3.SystemProgram.programId;
  await assert.rejects(
    invalid.observer.observeFinalized(invalid.record),
    (error: unknown) =>
      error instanceof Error && !(error instanceof SolanaUnavailableError),
  );
  const absent = await fixture(t);
  absent.accounts[4] = null;
  assert.deepEqual(await absent.observer.observeFinalized(absent.record), {
    kind: "absent",
    contextSlot: 110n,
  });
});
