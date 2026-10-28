import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";
import { BN, Program, utils, web3, type IdlAccounts } from "@coral-xyz/anchor";
import { MintLayout, TOKEN_PROGRAM_ID } from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import type { AktSolana } from "../target/types/akt_solana";
import Long from "long";
import {
  LightClientMismatchError,
  SolanaObserver,
  decodeRecord,
  defaultSlotLead,
} from "./solana";
import { planOutcome, reportedObservations, type Snapshot } from "./policy";
import {
  Outcome,
  Observation,
  Sink_Params as Params,
  Settlement,
  Sink_Status as Status,
  Totals,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";

async function fixture(t: TestContext) {
  // Every RPC method used is mocked. Port 9 keeps a missed mock off any node.
  const rpc = new web3.Connection("http://127.0.0.1:9");
  const program = new Program<AktSolana>(idl, { connection: rpc });
  const pda = (seed: string, suffix?: Buffer) =>
    web3.PublicKey.findProgramAddressSync(
      suffix ? [Buffer.from(seed), suffix] : [Buffer.from(seed)],
      program.programId,
    )[0];
  const config = {
    genesisHash: web3.Keypair.generate().publicKey.toBase58(),
    programId: program.programId,
    mint: pda("mint"),
    lightClientProgram: web3.Keypair.generate().publicKey,
    akashChainId: "akash-1",
    maxAgeSeconds: 60,
  };
  const raw = Buffer.alloc(77);
  raw[0] = 1;
  raw.writeBigUInt64LE(1n, 1);
  raw.writeBigUInt64LE(60n, 61);
  raw.writeBigUInt64LE(1n, 69);
  web3.Keypair.generate().publicKey.toBuffer().copy(raw, 29);
  const record = decodeRecord(raw);
  const id = Buffer.alloc(8);
  id.writeBigUInt64LE(record.id);
  const address = pda("transfer", id);
  const status: IdlAccounts<AktSolana>["transferStatus"] = {
    id: new BN(1),
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
  const bridgeAccount = async (lightClientProgram: web3.PublicKey) =>
    info(
      await program.coder.accounts.encode("bridgeConfig", {
        lightClientProgram,
        chainId: config.akashChainId,
        policy: {
          perTransferMax: new BN(100),
          dailyMintCap: new BN(1000),
          vaults: [],
        },
        mintDay: new BN(1),
        mintedToday: new BN(60),
      }),
    );
  const mintData = Buffer.alloc(MintLayout.span);
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
    mintData,
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
    await bridgeAccount(config.lightClientProgram),
    info(mintData, TOKEN_PROGRAM_ID),
    info(await program.coder.accounts.encode("transferStatus", status)),
  ];
  const signature = utils.bytes.bs58.encode(Buffer.alloc(64, 7));
  const event = program.idl.events?.find(
    (event) => event.name === "transferResolved",
  );
  assert.ok(event);
  const data = Buffer.concat([
    Buffer.from(event.discriminator),
    program.coder.types.encode("transferResolved", {
      id: status.id,
      proofHeight: status.proofHeight,
      outcome: status.outcome,
    }),
  ]);
  const tx = new web3.Transaction({
    feePayer: record.recipient,
    recentBlockhash: web3.Keypair.generate().publicKey.toBase58(),
  }).add(
    new web3.TransactionInstruction({
      programId: program.programId,
      keys: [{ pubkey: address, isWritable: true, isSigner: false }],
      data: Buffer.alloc(0),
    }),
  );
  const eventLog = `Program data: ${data.toString("base64")}`;
  const resolution: web3.VersionedTransactionResponse = {
    slot: 100,
    transaction: { signatures: [signature], message: tx.compileMessage() },
    meta: {
      err: null,
      fee: 5000,
      preBalances: [],
      postBalances: [],
      logMessages: [
        `Program ${program.programId} invoke [1]`,
        eventLog,
        `Program ${program.programId} success`,
      ],
    },
  };
  t.mock.method(rpc, "getGenesisHash", async () => config.genesisHash);
  t.mock.method(rpc, "getMultipleAccountsInfoAndContext", async () => ({
    context: { slot: 110 },
    value: accounts,
  }));
  t.mock.method(rpc, "getBlockTime", async () => Math.floor(Date.now() / 1000));
  // A failed replay in the same block references the status and emits the
  // event bytes before failing; it must not supply the signature.
  const replay = {
    transaction: {
      signatures: [utils.bytes.bs58.encode(Buffer.alloc(64, 9))],
      message: tx.compileMessage(),
    },
    meta: {
      err: { InstructionError: [0, "Custom"] },
      fee: 5000,
      preBalances: [],
      postBalances: [],
      logMessages: [
        `Program ${program.programId} invoke [1]`,
        eventLog,
        `Program ${program.programId} failed: custom program error: 0x0`,
      ],
    },
  };
  const transactions: Pick<
    web3.VersionedTransactionResponse,
    "transaction" | "meta"
  >[] = [replay, resolution];
  const block = {
    blockhash: web3.Keypair.generate().publicKey.toBase58(),
    previousBlockhash: web3.Keypair.generate().publicKey.toBase58(),
    parentSlot: 99,
    blockTime: null,
    transactions,
  };
  const getBlock = t.mock.method(rpc, "getBlock", async () => block);
  const history = t.mock.method(rpc, "getSignaturesForAddress", async () => []);
  const lookup = t.mock.method(rpc, "getTransaction", async () => resolution);
  return {
    rpc,
    config,
    record,
    accounts,
    bridgeAccount,
    resolution,
    status,
    program,
    eventLog,
    block,
    getBlock,
    history,
    lookup,
    raw,
    observer: new SolanaObserver(rpc, config),
  };
}

// SDK fromPartial reads bigint uint64 values as signed; use unsigned Longs.
function report(slot: string, outcome = Outcome.OUTCOME_VOIDED, fill = 5) {
  return Observation.fromPartial({
    outcome,
    slot: Long.fromString(slot, true),
    transactionSignature: new Uint8Array(64).fill(fill),
  });
}

// A void reached threshold and is inside its challenge window.
function voided(raw: Buffer, observations: Observation[]): Snapshot {
  const members = ["reporter", "b", "c", "d"];
  return {
    params: Params.fromPartial({
      reporters: members,
      reporterThreshold: 3,
      refundsEnabled: true,
      dailyRefundCap: 100n,
    }),
    totals: Totals.fromPartial({ reporterEpoch: 1n, refundDay: 1n }),
    time: 86409n,
    transfer: {
      record: raw,
      voidOrdered: false,
      settlement: Settlement.fromPartial({
        round: 1n,
        status: Status.STATUS_VOIDED,
        challengeEnd: 86410n,
        policy: { epoch: 1n, members, threshold: 3 },
        votes: observations.map((observation, i) => ({
          reporter: members[i + 1],
          observation,
        })),
      }),
    },
  };
}

test("observer chooses the successful resolution over a failed replay", async (t) => {
  const f = await fixture(t);
  const result = await f.observer.observe(f.record, 100n);
  assert.ok(result.kind === "resolved");
  assert.equal(result.observation.slot.toBigInt(), 100n);
  assert.equal(result.observation.outcome, Outcome.OUTCOME_MINTED);
  assert.deepEqual(
    result.observation.transactionSignature,
    new Uint8Array(64).fill(7),
  );
  assert.equal(f.getBlock.mock.calls[0]?.arguments[0], 100);
  assert.deepEqual(f.getBlock.mock.calls[0]?.arguments[1], {
    commitment: "finalized",
    maxSupportedTransactionVersion: 1,
    transactionDetails: "full",
    rewards: false,
  });
});

test("a version 1 resolution transaction is evidence", async (t) => {
  const f = await fixture(t);
  const legacy = f.resolution.transaction.message;
  // SIMD-0385 messages carry static keys only, with the same header rules.
  f.resolution.transaction.message = new web3.MessageV1({
    header: legacy.header,
    staticAccountKeys: legacy.staticAccountKeys,
    recentBlockhash: legacy.recentBlockhash,
    compiledInstructions: legacy.compiledInstructions,
  });
  f.resolution.version = 1;
  const result = await f.observer.observe(f.record, 100n);
  assert.ok(result.kind === "resolved");
  assert.equal(result.observation.outcome, Outcome.OUTCOME_MINTED);
  assert.deepEqual(
    result.observation.transactionSignature,
    new Uint8Array(64).fill(7),
  );
});

test("spam referencing the status account cannot hide the resolution", async (t) => {
  const f = await fixture(t);
  const meta = f.resolution.meta;
  assert.ok(meta);
  const spam = Array.from({ length: 2_000 }, (_, i) => ({
    ...f.resolution,
    transaction: {
      ...f.resolution.transaction,
      signatures: [
        utils.bytes.bs58.encode(
          Buffer.concat([
            Buffer.alloc(60, 1),
            Buffer.from(Uint32Array.of(i).buffer),
          ]),
        ),
      ],
    },
    meta: {
      ...meta,
      logMessages: [
        `Program ${f.program.programId} invoke [1]`,
        `Program ${f.program.programId} success`,
      ],
    },
  }));
  f.block.transactions = [...spam, f.resolution, ...spam];
  const result = await f.observer.observe(f.record);
  assert.ok(result.kind === "resolved");
  assert.deepEqual(
    result.observation.transactionSignature,
    new Uint8Array(64).fill(7),
  );
  assert.equal(f.history.mock.callCount(), 0);
});

test("two resolutions for one status fail closed", async (t) => {
  const f = await fixture(t);
  f.block.transactions = [
    f.resolution,
    {
      ...f.resolution,
      transaction: {
        ...f.resolution.transaction,
        signatures: [utils.bytes.bs58.encode(Buffer.alloc(64, 8))],
      },
    },
  ];
  await assert.rejects(
    () => f.observer.observe(f.record),
    /multiple resolution transactions/,
  );
});

test("observer rejects wrong cluster, status owner, amount, recipient and stale state", async (t) => {
  for (const failure of [
    "cluster",
    "owner",
    "amount",
    "recipient",
    "stale",
    "event",
    "failed",
  ]) {
    await t.test(failure, async (t) => {
      const f = await fixture(t);
      if (failure === "cluster") f.config.genesisHash = "wrong";
      if (failure === "cluster")
        t.mock.method(f.rpc, "getGenesisHash", async () => "different");
      if (failure === "owner") {
        assert.ok(f.accounts[4]);
        f.accounts[4].owner = web3.SystemProgram.programId;
      }
      if (failure === "amount") f.record.amount++;
      if (failure === "recipient")
        f.record.recipient = web3.Keypair.generate().publicKey;
      if (failure === "stale")
        t.mock.method(f.rpc, "getBlockTime", async () => 1);
      if (failure === "event") {
        assert.ok(f.resolution.meta);
        f.resolution.meta.logMessages = [];
      }
      if (failure === "failed") {
        assert.ok(f.resolution.meta);
        f.resolution.meta.err = { InstructionError: [0, "InvalidArgument"] };
      }
      await assert.rejects(() => f.observer.observe(f.record));
    });
  }
});

test("events emitted inside a CPI count only from an AKT program frame", async (t) => {
  const wrapper = web3.Keypair.generate().publicKey.toBase58();
  const token = TOKEN_PROGRAM_ID.toBase58();
  const cases: [string, (p: string, e: string) => string[], boolean][] = [
    [
      "AKT frame inside a wrapper CPI",
      (p, e) => [
        `Program ${wrapper} invoke [1]`,
        `Program ${p} invoke [2]`,
        `Program ${token} invoke [3]`,
        `Program ${token} success`,
        e,
        `Program ${p} consumed 1 of 2 compute units`,
        `Program ${p} success`,
        `Program ${wrapper} success`,
      ],
      true,
    ],
    [
      "AKT frame after a nested call returns",
      (p, e) => [
        `Program ${p} invoke [1]`,
        `Program ${token} invoke [2]`,
        `Program ${token} success`,
        e,
        `Program ${p} success`,
      ],
      true,
    ],
    [
      "wrapper frame replaying AKT event bytes",
      (p, e) => [
        `Program ${wrapper} invoke [1]`,
        `Program ${p} invoke [2]`,
        `Program ${p} success`,
        e,
        `Program ${wrapper} success`,
      ],
      false,
    ],
    [
      "nested program frame",
      (p, e) => [
        `Program ${p} invoke [1]`,
        `Program ${token} invoke [2]`,
        e,
        `Program ${token} success`,
        `Program ${p} success`,
      ],
      false,
    ],
    [
      "program log line",
      (p, e) => [
        `Program ${p} invoke [1]`,
        e.replace("Program data: ", "Program log: "),
        `Program ${p} success`,
      ],
      false,
    ],
  ];
  for (const [name, logs, accepted] of cases) {
    await t.test(name, async (t) => {
      const f = await fixture(t);
      assert.ok(f.resolution.meta);
      f.resolution.meta.logMessages = logs(
        f.program.programId.toBase58(),
        f.eventLog,
      );
      if (accepted) {
        const result = await f.observer.observe(f.record);
        assert.ok(result.kind === "resolved");
        assert.deepEqual(
          result.observation.transactionSignature,
          new Uint8Array(64).fill(7),
        );
      } else {
        await assert.rejects(
          () => f.observer.observe(f.record),
          /evidence unavailable/,
        );
      }
    });
  }
});

test("truncated or unbalanced logs never supply evidence", async (t) => {
  const other = web3.Keypair.generate().publicKey.toBase58();
  const cases: [string, (p: string, e: string) => string[]][] = [
    ["truncated", (p, e) => [`Program ${p} invoke [1]`, e, "Log truncated"]],
    ["unterminated", (p, e) => [`Program ${p} invoke [1]`, e]],
    [
      "wrong depth",
      (p, e) => [`Program ${p} invoke [2]`, e, `Program ${p} success`],
    ],
    [
      "mismatched exit",
      (p, e) => [`Program ${p} invoke [1]`, e, `Program ${other} success`],
    ],
    [
      "failed frame",
      (p, e) => [
        `Program ${p} invoke [1]`,
        e,
        `Program ${p} failed: custom program error: 0x1`,
      ],
    ],
    [
      "data outside a frame",
      (p, e) => [`Program ${p} invoke [1]`, `Program ${p} success`, e],
    ],
  ];
  for (const [name, logs] of cases) {
    await t.test(name, async (t) => {
      const f = await fixture(t);
      assert.ok(f.resolution.meta);
      f.resolution.meta.logMessages = logs(
        f.program.programId.toBase58(),
        f.eventLog,
      );
      await assert.rejects(
        () => f.observer.observe(f.record),
        /evidence unavailable/,
      );
    });
  }
});

test("log truncation keeps only top-level AKT frames that already succeeded", async (t) => {
  const budget = web3.ComputeBudgetProgram.programId.toBase58();
  const other = web3.Keypair.generate().publicKey.toBase58();
  const cases: [string, (p: string, e: string) => string[], boolean][] = [
    [
      "after a completed AKT frame",
      (p, e) => [
        `Program ${budget} invoke [1]`,
        `Program ${budget} success`,
        `Program ${p} invoke [1]`,
        `Program ${other} invoke [2]`,
        `Program ${other} success`,
        e,
        `Program ${p} consumed 1 of 2 compute units`,
        `Program ${p} success`,
        `Program ${other} invoke [1]`,
        "Program log: padding",
        "Log truncated",
      ],
      true,
    ],
    [
      "inside the AKT frame",
      (p, e) => [
        `Program ${p} invoke [1]`,
        e,
        `Program ${other} invoke [2]`,
        "Log truncated",
      ],
      false,
    ],
    [
      "before the AKT frame",
      () => [
        `Program ${budget} invoke [1]`,
        `Program ${budget} success`,
        `Program ${other} invoke [1]`,
        "Log truncated",
      ],
      false,
    ],
    [
      "lines after the truncation marker",
      (p, e) => [
        `Program ${budget} invoke [1]`,
        `Program ${budget} success`,
        `Program ${other} invoke [1]`,
        "Log truncated",
        `Program ${p} invoke [1]`,
        e,
        `Program ${p} success`,
      ],
      false,
    ],
    [
      "completed AKT frame without the event",
      (p) => [
        `Program ${p} invoke [1]`,
        `Program ${p} success`,
        "Log truncated",
      ],
      false,
    ],
    [
      "AKT event inside a completed wrapper frame",
      (p, e) => [
        `Program ${other} invoke [1]`,
        `Program ${p} invoke [2]`,
        e,
        `Program ${p} success`,
        `Program ${other} success`,
        "Log truncated",
      ],
      false,
    ],
  ];
  for (const [name, logs, accepted] of cases) {
    await t.test(name, async (t) => {
      const f = await fixture(t);
      assert.ok(f.resolution.meta);
      f.resolution.meta.logMessages = logs(
        f.program.programId.toBase58(),
        f.eventLog,
      );
      if (accepted) {
        const result = await f.observer.observe(f.record);
        assert.ok(result.kind === "resolved");
        assert.deepEqual(
          result.observation.transactionSignature,
          new Uint8Array(64).fill(7),
        );
        // A failed transaction still supplies nothing.
        f.resolution.meta.err = { InstructionError: [2, "Custom"] };
      }
      await assert.rejects(
        () => f.observer.observe(f.record),
        /evidence unavailable/,
      );
    });
  }
});

test("missing status stays absent; pruned transaction evidence never produces a report", async (t) => {
  const f = await fixture(t);
  for (const missing of [
    async () => {
      throw new Error("Block 100 cleaned up, does not exist on node");
    },
    async () => null,
  ]) {
    t.mock.method(f.rpc, "getBlock", missing);
    await assert.rejects(
      () => f.observer.observe(f.record),
      /evidence unavailable: block missing/,
    );
  }
  t.mock.method(f.rpc, "getBlock", async () => ({
    ...f.block,
    transactions: [],
  }));
  await assert.rejects(
    () => f.observer.observe(f.record),
    /evidence unavailable in its block/,
  );
  f.accounts[4] = null;
  await assert.rejects(
    () => f.observer.observe(f.record, 111n),
    /behind reported evidence/,
  );
  assert.deepEqual(await f.observer.observe(f.record), {
    kind: "absent",
    contextSlot: 110n,
    slotLimit: 110n + BigInt(defaultSlotLead(60)),
  });
});

test("a fabricated slot beyond any plausible finalized slot cannot block a veto", async (t) => {
  const f = await fixture(t);
  f.accounts[4] = null;
  const snapshot = voided(
    f.raw,
    ["b", "c", "d"].map(() => report("9223372036854775808")),
  );
  const observed = await f.observer.observe(
    f.record,
    reportedObservations(snapshot.transfer),
  );
  assert.deepEqual(observed, {
    kind: "absent",
    contextSlot: 110n,
    slotLimit: 710n,
  });
  assert.equal(f.lookup.mock.callCount(), 0);
  const plan = planOutcome("reporter", snapshot, observed);
  assert.equal(plan.messages[0]?.typeUrl, "/akash.sink.v1.MsgVetoVoid");
  assert.match(plan.alerts.join(), /beyond any plausible finalized slot/);
  // The legacy single-slot bound gets the same plausibility limit.
  assert.deepEqual(
    await f.observer.observe(f.record, 9_223_372_036_854_775_808n),
    observed,
  );
  // A configured lead replaces the default derived from maxAgeSeconds.
  const strict = new SolanaObserver(f.rpc, { ...f.config, maxSlotLead: 50 });
  const near = voided(f.raw, [report("161"), report("161"), report("161")]);
  assert.equal(
    planOutcome(
      "reporter",
      near,
      await strict.observe(f.record, reportedObservations(near.transfer)),
    ).messages[0]?.typeUrl,
    "/akash.sink.v1.MsgVetoVoid",
  );
  assert.throws(
    () => new SolanaObserver(f.rpc, { ...f.config, maxSlotLead: 0 }),
    /invalid maxSlotLead/,
  );
});

test("a lagging but fresh node never vetoes a void it has not reached", async (t) => {
  const f = await fixture(t);
  f.accounts[4] = null;
  for (const slots of [
    ["111", "111", "111"],
    ["710", "710", "710"],
    ["9223372036854775808", "400", "400"],
  ]) {
    const snapshot = voided(
      f.raw,
      slots.map((slot) => report(slot)),
    );
    await assert.rejects(
      () =>
        f.observer.observe(f.record, reportedObservations(snapshot.transfer)),
      /behind reported evidence/,
    );
  }
  assert.equal(f.lookup.mock.callCount(), 0);
});

test("a reported void signature missing at a passed slot is vetoed", async (t) => {
  const f = await fixture(t);
  f.accounts[4] = null;
  const snapshot = voided(f.raw, [
    report("100", Outcome.OUTCOME_VOIDED, 7),
    report("105"),
    report("110"),
  ]);
  for (const response of [null, f.resolution]) {
    // Neither a missing transaction nor the mint at slot 100 shows a void.
    t.mock.method(f.rpc, "getTransaction", async () => response);
    const observed = await f.observer.observe(
      f.record,
      reportedObservations(snapshot.transfer),
    );
    assert.ok(observed.kind === "absent");
    assert.equal(
      planOutcome("reporter", snapshot, observed).messages[0]?.typeUrl,
      "/akash.sink.v1.MsgVetoVoid",
    );
  }
  // A finalized transaction that does show the reported outcome means this
  // node's missing status is inconsistent; it must not support a veto.
  const lookup = t.mock.method(
    f.rpc,
    "getTransaction",
    async () => f.resolution,
  );
  await assert.rejects(
    () =>
      f.observer.observe(f.record, [
        report("100"),
        report("100", Outcome.OUTCOME_MINTED, 7),
      ]),
    /contradicts missing status/,
  );
  assert.equal(
    lookup.mock.calls[1]?.arguments[0],
    utils.bytes.bs58.encode(Buffer.alloc(64, 7)),
  );
  // The same transaction reported at another slot is false evidence.
  assert.equal(
    (
      await f.observer.observe(f.record, [
        report("101", Outcome.OUTCOME_MINTED, 7),
      ])
    ).kind,
    "absent",
  );
  t.mock.method(f.rpc, "getTransaction", async () => {
    throw new Error("connection reset");
  });
  await assert.rejects(
    () => f.observer.observe(f.record, [report("100")]),
    /temporarily unavailable/,
  );
});

test("a stale node never vetoes, even against a fabricated slot", async (t) => {
  const f = await fixture(t);
  f.accounts[4] = null;
  t.mock.method(f.rpc, "getBlockTime", async () => 1);
  const snapshot = voided(
    f.raw,
    ["b", "c", "d"].map(() => report("9223372036854775808")),
  );
  await assert.rejects(
    () => f.observer.observe(f.record, reportedObservations(snapshot.transfer)),
    /stale Solana/,
  );
});

test("a fabricated future report slot cannot hide an existing finalized mint", async (t) => {
  const f = await fixture(t);
  const observed = await f.observer.observe(
    f.record,
    18_446_744_073_709_551_615n,
  );
  assert.ok(observed.kind === "resolved");
  assert.equal(observed.observation.outcome, Outcome.OUTCOME_MINTED);
});

test("own stored evidence survives pruned history while finalized status is rechecked", async (t) => {
  const f = await fixture(t);
  const initial = await f.observer.observe(f.record);
  assert.ok(initial.kind === "resolved");
  const history = t.mock.method(f.rpc, "getBlock", async () => null);
  assert.deepEqual(
    await f.observer.observe(f.record, 0n, initial.observation),
    initial,
  );
  assert.equal(history.mock.callCount(), 0);
  f.record.amount++;
  await assert.rejects(
    () => f.observer.observe(f.record, 0n, initial.observation),
    /mint amount differs/,
  );
  f.record.amount--;
  t.mock.method(f.rpc, "getBlockTime", async () => 1);
  await assert.rejects(
    () => f.observer.observe(f.record, 0n, initial.observation),
    /stale Solana/,
  );
});

test("different or malformed stored evidence cannot replace transaction discovery", async (t) => {
  const f = await fixture(t);
  const initial = await f.observer.observe(f.record);
  assert.ok(initial.kind === "resolved");
  t.mock.method(f.rpc, "getBlock", async () => null);
  for (const change of [
    { slot: 101n },
    { outcome: Outcome.OUTCOME_VOIDED },
    { transactionSignature: new Uint8Array(63).fill(7) },
    { transactionSignature: new Uint8Array(64) },
  ]) {
    await assert.rejects(
      () =>
        f.observer.observe(
          f.record,
          0n,
          Observation.fromPartial({ ...initial.observation, ...change }),
        ),
      /evidence unavailable/,
    );
  }
});

test("uint64 deposit amounts and IDs are decoded without floating point rounding", () => {
  const raw = Buffer.alloc(77);
  raw[0] = 1;
  raw.writeBigUInt64LE(18_446_744_073_709_551_615n, 1);
  raw.writeBigUInt64LE(9_007_199_254_740_993n, 61);
  raw.writeBigUInt64LE(1n, 69);
  assert.equal(decodeRecord(raw).amount, 9_007_199_254_740_993n);
  assert.equal(decodeRecord(raw).id, 18_446_744_073_709_551_615n);
});

test("a pinned light client fails closed when governance re-pins", async (t) => {
  const f = await fixture(t);
  assert.ok(f.config.lightClientProgram);
  assert.ok(
    (await f.observer.lightClient()).equals(f.config.lightClientProgram),
  );
  const repinned = web3.Keypair.generate().publicKey;
  f.accounts[2] = await f.bridgeAccount(repinned);
  for (const read of [
    () => f.observer.lightClient(),
    () => f.observer.observe(f.record),
    () => f.observer.observeFinalized(f.record),
  ])
    await assert.rejects(read, (error: unknown) => {
      assert.ok(error instanceof LightClientMismatchError);
      assert.ok(error.current.equals(repinned));
      assert.ok(error.pinned.equals(f.config.lightClientProgram!));
      return true;
    });
  assert.equal(f.getBlock.mock.callCount(), 0);
  assert.deepEqual(f.observer.takeLightClientChanges(), []);
});

test("an unpinned observer follows the bridge light client and records changes", async (t) => {
  const f = await fixture(t);
  const first = f.config.lightClientProgram;
  assert.ok(first);
  const observer = new SolanaObserver(f.rpc, {
    ...f.config,
    lightClientProgram: undefined,
  });
  assert.ok((await observer.lightClient()).equals(first));
  assert.ok((await observer.observe(f.record)).kind === "resolved");
  assert.deepEqual(observer.takeLightClientChanges(), []);
  const second = web3.Keypair.generate().publicKey;
  f.accounts[2] = await f.bridgeAccount(second);
  const observed = await observer.observe(f.record);
  assert.ok(observed.kind === "resolved");
  assert.equal(observed.observation.outcome, Outcome.OUTCOME_MINTED);
  assert.ok((await observer.lightClient()).equals(second));
  const changes = observer.takeLightClientChanges();
  assert.equal(changes.length, 1);
  assert.ok(changes[0]?.previous.equals(first));
  assert.ok(changes[0]?.current.equals(second));
  assert.equal(changes[0]?.contextSlot, 110n);
  assert.deepEqual(observer.takeLightClientChanges(), []);
  // An older finalized snapshot cannot move the followed program back.
  const original = [...f.accounts.slice(0, 2), await f.bridgeAccount(first)];
  const at = (slot: number) =>
    t.mock.method(f.rpc, "getMultipleAccountsInfoAndContext", async () => ({
      context: { slot },
      value: [...original, f.accounts[3]],
    }));
  at(105);
  assert.ok((await observer.lightClient()).equals(first));
  assert.deepEqual(observer.takeLightClientChanges(), []);
  at(120);
  await observer.lightClient();
  assert.deepEqual(
    observer
      .takeLightClientChanges()
      .map((change) => [
        change.previous.toBase58(),
        change.current.toBase58(),
        change.contextSlot,
      ]),
    [[second.toBase58(), first.toBase58(), 120n]],
  );
});
