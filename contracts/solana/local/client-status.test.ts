import assert from "node:assert/strict";
import { rm } from "node:fs/promises";
import test, { type TestContext } from "node:test";
import { AnchorProvider, BN, Wallet, web3 } from "@coral-xyz/anchor";
import { assertClientActive } from "./client-status";
import { pda } from "./common";
import { clientProgram, discoverLightClient } from "./light-client";
import { relayContext } from "./relay";
import {
  CHAIN_ID,
  FakeSolana,
  NOW,
  relayTestConfig,
  testBridge,
} from "./relay-test-fixture";
import { temporaryDirectory } from "./test-workspace";

async function fixture(t: TestContext) {
  const stateDir = await temporaryDirectory("client-status-");
  t.after(() => rm(stateDir, { recursive: true, force: true }));
  const connection = new FakeSolana();
  const wallet = new AnchorProvider(
    connection,
    new Wallet(web3.Keypair.generate()),
    { commitment: "confirmed" },
  );
  const context = await relayContext({
    config: relayTestConfig(stateDir),
    wallet,
  });
  t.after(() => context.close());
  await testBridge(connection, context);
  const ids = await discoverLightClient(connection);
  const client = clientProgram(wallet, ids.program);
  return {
    connection,
    wallet,
    client,
    check: () => assertClientActive(wallet, client, CHAIN_ID),
  };
}

test("active client remains usable when the RPC cannot provide block time", async (t) => {
  const { connection, check } = await fixture(t);
  const rpc: web3.Connection = connection;
  t.mock.method(rpc, "getBlockTime", async () => null);
  await assert.doesNotReject(check);
  t.mock.method(rpc, "getBlockTime", async () => {
    throw new Error("Block not available for slot");
  });
  await assert.doesNotReject(check);
});

test("expiry follows the Clock account and rejects the exact deadline", async (t) => {
  const { connection, check } = await fixture(t);
  const expiry = BigInt(NOW) + 86_400n;
  const clock = Buffer.alloc(40);
  clock.writeBigInt64LE(expiry - 1n, 32);
  connection.set(web3.SYSVAR_CLOCK_PUBKEY, web3.PublicKey.default, clock);
  await assert.doesNotReject(check);

  clock.writeBigInt64LE(expiry, 32);
  await assert.rejects(check, /light client is expired/);
  clock.writeBigInt64LE(expiry + 1n, 32);
  await assert.rejects(check, /light client is expired/);
});

test("Clock lookup preserves the frozen-client rejection", async (t) => {
  const { connection, client, check } = await fixture(t);
  const address = pda("client", client.programId);
  const state = await client.account.clientState.fetch(address);
  state.frozenHeight.revisionHeight = new BN(1);
  connection.set(
    address,
    client.programId,
    await client.coder.accounts.encode("clientState", state),
  );
  await assert.rejects(check, /light client is frozen/);
});

test("missing or malformed Clock accounts stop uploads", async (t) => {
  const { connection, check } = await fixture(t);
  connection.accounts.delete(web3.SYSVAR_CLOCK_PUBKEY.toBase58());
  await assert.rejects(check, /Solana clock unavailable/);
  for (const length of [0, 39, 41]) {
    connection.set(
      web3.SYSVAR_CLOCK_PUBKEY,
      web3.PublicKey.default,
      Buffer.alloc(length),
    );
    await assert.rejects(check, /Solana clock unavailable/);
  }
});

test("Clock is read at confirmed commitment and RPC failures stop uploads", async (t) => {
  const { connection, check } = await fixture(t);
  const readAccount = connection.getAccountInfo.bind(connection);
  const rpc: web3.Connection = connection;
  let unavailable = false;
  t.mock.method(
    rpc,
    "getAccountInfo",
    async (
      address: web3.PublicKey,
      commitment?: web3.Commitment | web3.GetAccountInfoConfig,
    ) => {
      if (address.equals(web3.SYSVAR_CLOCK_PUBKEY)) {
        assert.equal(commitment, "confirmed");
        if (unavailable) throw new Error("Clock RPC unavailable");
      }
      return readAccount(address);
    },
  );
  await assert.doesNotReject(check);
  unavailable = true;
  await assert.rejects(check, /Clock RPC unavailable/);
});

test("a different Akash chain remains rejected", async (t) => {
  const { wallet, client } = await fixture(t);
  await assert.rejects(() =>
    assertClientActive(wallet, client, "another-akash-chain"),
  );
});
