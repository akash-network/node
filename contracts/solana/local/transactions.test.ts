import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile, rm } from "node:fs/promises";
import { join } from "node:path";
import test, { type TestContext } from "node:test";
import { temporaryDirectory } from "./test-workspace";
import { AnchorProvider, Wallet, web3 } from "@coral-xyz/anchor";
import { optionalJson } from "./durable";
import { object } from "./common";
import { resumeTransaction, sendTransaction } from "./transactions";

async function journalRecovery(
  t: TestContext,
  error: web3.TransactionError | null,
) {
  const payer = web3.Keypair.generate();
  const genesis = web3.Keypair.generate().publicKey.toBase58();
  let blockhash = web3.Keypair.generate().publicKey.toBase58();
  let blockHeight = 10;
  const directory = await temporaryDirectory("sink-v2-transaction-");
  t.after(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, "pending.json");
  let broadcasts = 0,
    simulations = 0;
  let simulationFails = false,
    unavailable = false;
  let signature: string | undefined;
  const server = createServer(async (request, response) => {
    try {
      const parts: Buffer[] = [];
      for await (const part of request) parts.push(Buffer.from(part));
      const body = object(JSON.parse(Buffer.concat(parts).toString()));
      const params = body.params;
      assert.ok(Array.isArray(params));
      let result: unknown;
      switch (body.method) {
        case "getGenesisHash":
          result = genesis;
          break;
        case "getLatestBlockhash":
          result = {
            context: { slot: 10 },
            value: { blockhash, lastValidBlockHeight: 1000 },
          };
          break;
        case "getBlockHeight":
          result = blockHeight;
          break;
        case "simulateTransaction":
          simulations++;
          assert.equal(await optionalJson(path), undefined);
          result = {
            context: { slot: 10 },
            value: {
              err: simulationFails
                ? { InstructionError: [0, "InvalidArgument"] }
                : null,
              logs: [],
              accounts: null,
              unitsConsumed: 10,
            },
          };
          break;
        case "sendTransaction": {
          const saved = object(await optionalJson(path));
          assert.equal(
            saved.raw,
            params[0],
            "signed intent must exist before the first broadcast",
          );
          assert.equal(simulations, 1);
          assert.ok(typeof saved.signature === "string");
          signature = saved.signature;
          broadcasts++;
          result = signature;
          break;
        }
        case "getSignatureStatuses":
          result = {
            context: { slot: 20 },
            value: [
              signature
                ? {
                    slot: 12,
                    confirmations: null,
                    err: error,
                    confirmationStatus: "finalized",
                  }
                : null,
            ],
          };
          break;
        case "getTransaction":
          if (unavailable) throw new Error("history unavailable");
          result = {
            slot: 12,
            blockTime: 100,
            version: 0,
            transaction: {
              signatures: [signature],
              message: {
                header: {
                  numRequiredSignatures: 1,
                  numReadonlySignedAccounts: 0,
                  numReadonlyUnsignedAccounts: 0,
                },
                accountKeys: [payer.publicKey.toBase58()],
                recentBlockhash: blockhash,
                instructions: [],
                addressTableLookups: [],
              },
            },
            meta: {
              err: error,
              fee: 5000,
              preBalances: [100000],
              postBalances: [95000],
              logMessages: [],
              loadedAddresses: { writable: [], readonly: [] },
            },
          };
          break;
        default:
          throw new Error(`unexpected RPC ${body.method}`);
      }
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ jsonrpc: "2.0", id: body.id, result }));
    } catch {
      response.statusCode = 503;
      response.end("test RPC unavailable");
    }
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(
    () =>
      new Promise<void>((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      ),
  );
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  const wallet = new AnchorProvider(
    new web3.Connection(`http://127.0.0.1:${address.port}`, "confirmed"),
    new Wallet(payer),
    { commitment: "confirmed" },
  );
  await assert.rejects(
    () =>
      sendTransaction({
        wallet,
        stage: "resolve",
        instructions: [],
        pendingPath: path,
        preflight: true,
        observe: async () => {
          throw new Error("crash before durable charge");
        },
      }),
    /crash before durable charge/,
  );
  assert.equal(broadcasts, 1);
  const saved = await readFile(path, "utf8");
  unavailable = true;
  await assert.rejects(() =>
    resumeTransaction(wallet, path, async () =>
      assert.fail("unavailable history cannot settle spending"),
    ),
  );
  assert.equal(await readFile(path, "utf8"), saved);
  unavailable = false;
  const charged: string[] = [];
  await resumeTransaction(wallet, path, async (result) => {
    assert.equal(result.fee, 5000);
    assert.equal(result.payerLamports, 5000);
    assert.deepEqual(result.error, error);
    charged.push(result.signature);
  });
  await resumeTransaction(wallet, path, async () =>
    assert.fail("a settled transaction cannot be charged again"),
  );
  assert.deepEqual(charged, [signature]);
  assert.equal(await optionalJson(path), undefined);
  assert.equal(
    broadcasts,
    1,
    "restart must not replace an already finalized transaction",
  );
  simulationFails = true;
  await assert.rejects(
    () =>
      sendTransaction({
        wallet,
        stage: "proof-create",
        instructions: [],
        pendingPath: path,
        preflight: true,
        observe: async () => assert.fail("failed simulation cannot charge"),
      }),
    /simulation failed/,
  );
  assert.equal(broadcasts, 1);
  assert.equal(await optionalJson(path), undefined);
  blockhash = web3.Keypair.generate().publicKey.toBase58();
  blockHeight = 2000;
  signature = undefined;
  await assert.rejects(
    () =>
      sendTransaction({
        wallet,
        stage: "proof-append",
        instructions: [],
        pendingPath: path,
        observe: async () => assert.fail("unresolved history cannot charge"),
      }),
    /history unresolved.*intent retained/,
  );
  const expired = await readFile(path, "utf8");
  await assert.rejects(
    () =>
      resumeTransaction(wallet, path, async () =>
        assert.fail("unresolved history cannot settle"),
      ),
    /history unresolved.*intent retained/,
  );
  assert.equal(await readFile(path, "utf8"), expired);
  assert.equal(broadcasts, 1);
}

test("landed success journals before broadcast and observes accounting once across restart", async (t) => {
  await journalRecovery(t, null);
});

test("landed failure journals before broadcast and observes accounting once across restart", async (t) => {
  await journalRecovery(t, { InstructionError: [0, "InvalidArgument"] });
});
