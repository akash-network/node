import assert from "node:assert/strict";
import { readFile, readdir, rm } from "node:fs/promises";
import { join } from "node:path";
import test, { type TestContext } from "node:test";
import { temporaryDirectory } from "./test-workspace";
import {
  AnchorProvider,
  BorshInstructionCoder,
  BN,
  Wallet,
  utils,
  web3,
} from "@coral-xyz/anchor";
import {
  EpochUsage,
  QueryStateResponse,
  ReporterPolicy,
  SettlementData,
  Sink_Params as Params,
  Sink_Status as Status,
  Totals,
  TransferState,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { CommitmentProof } from "cosmjs-types/cosmos/ics23/v1/proofs";
import { MerkleProof } from "cosmjs-types/ibc/core/commitment/v1/commitment";
import { testTransfer } from "../tests/sink-fixtures";
import { parseSinkId } from "../shared/id";
import {
  anchorId,
  encodeRecord,
  proofAddress,
  settlementKey,
  statusAddress,
  transferKey,
} from "../shared/proof";
import { object, PROGRAM_ID } from "./common";
import { relayContext } from "./relay";
import {
  FakeSolana,
  testBridge,
  relayTestConfig,
  HEIGHT,
  NOW,
  TRUSTED,
} from "./relay-test-fixture";

// RPC boundary model. It exercises the real tick, signed transaction journal,
// fee accounting and Anchor instruction encoding, not SBF proof verification.
class PayingSolana extends FakeSolana {
  context?: Awaited<ReturnType<typeof relayContext>>;
  readonly transactions = new Map<string, web3.VersionedTransactionResponse>();
  readonly actions: {
    name: string;
    address: string;
    offset?: number;
    key?: string;
  }[] = [];
  balance = 1_000_000_000;
  crashAfter?: string;
  failNext?: string;
  afterAction?: (name: string) => void;
  override async getLatestBlockhash() {
    return {
      blockhash: web3.Keypair.generate().publicKey.toBase58(),
      lastValidBlockHeight: 1000,
    };
  }
  override getBlockHeight = async () => 100;
  override async getSignatureStatuses(signatures: string[]) {
    return {
      context: { slot: 1 },
      value: signatures.map((signature) =>
        this.transactions.has(signature)
          ? {
              slot: 1,
              confirmations: null,
              err: this.transactions.get(signature)?.meta?.err ?? null,
              confirmationStatus: "finalized" as const,
            }
          : null,
      ),
    };
  }
  override getTransaction(
    signature: string,
    config?: web3.GetTransactionConfig,
  ): Promise<web3.TransactionResponse | null>;
  override getTransaction(
    signature: string,
    config: web3.GetVersionedTransactionConfig,
  ): Promise<web3.VersionedTransactionResponse | null>;
  override async getTransaction(
    signature: string,
    config?: web3.GetVersionedTransactionConfig,
  ): Promise<
    web3.TransactionResponse | web3.VersionedTransactionResponse | null
  > {
    assert.equal(config?.maxSupportedTransactionVersion, 0);
    return this.transactions.get(signature) ?? null;
  }
  override async simulateTransaction() {
    return {
      context: { slot: 1 },
      value: { err: null, logs: [], unitsConsumed: 1 },
    };
  }
  override async sendRawTransaction(raw: Buffer | Uint8Array | number[]) {
    assert.ok(this.context);
    const { program, wallet } = this.context;
    const tx = web3.VersionedTransaction.deserialize(Uint8Array.from(raw));
    const signature = utils.bytes.bs58.encode(tx.signatures[0]);
    if (this.transactions.has(signature)) return signature;
    const before = this.balance;
    this.balance -= 5000;
    let action = "";
    let error: web3.TransactionError | null = null;
    for (const ix of tx.message.compiledInstructions) {
      if (!tx.message.staticAccountKeys[ix.programIdIndex].equals(PROGRAM_ID))
        continue;
      const instruction = new BorshInstructionCoder(program.idl).decode(
        Buffer.from(ix.data),
      );
      assert.ok(instruction);
      const args = object(instruction.data),
        name = instruction.name;
      const keys = ix.accountKeyIndexes.map(
        (index) => tx.message.staticAccountKeys[index],
      );
      const address =
        keys[name === "voidTransfer" ? 2 : name === "mintTransfer" ? 3 : 1];
      action = name;
      if (this.failNext === name) {
        this.failNext = undefined;
        error = { InstructionError: [0, "InvalidArgument"] };
        break;
      }
      if (name === "createProofBuffer") {
        assert.equal(
          await this.getAccountInfo(address),
          null,
          "duplicate create",
        );
        assert.ok(
          typeof args.expectedLen === "number" && Array.isArray(args.nonce),
        );
        const length = Math.min(80 + args.expectedLen, 10240),
          rent = (length + 128) * 6960;
        const data = await program.coder.accounts.encode("proofBuffer", {
          submitter: wallet.publicKey,
          nonce: args.nonce,
          expectedLen: args.expectedLen,
          data: Buffer.alloc(0),
        });
        this.set(
          address,
          PROGRAM_ID,
          Buffer.concat([data, Buffer.alloc(length - data.length)]),
        );
        const account = await this.getAccountInfo(address);
        assert.ok(account);
        account.lamports = rent;
        this.balance -= rent;
      } else if (name === "growProofBuffer" || name === "appendProof") {
        const account = await this.getAccountInfo(address);
        assert.ok(account);
        const state = await program.account.proofBuffer.fetch(address);
        let length = account.data.length;
        if (name === "growProofBuffer") {
          length = Math.min(80 + state.expectedLen, length + 10240);
          const rent = (length + 128) * 6960;
          this.balance -= rent - account.lamports;
          account.lamports = rent;
        } else {
          assert.equal(
            args.offset,
            state.data.length,
            "append skipped or replayed bytes",
          );
          assert.ok(Buffer.isBuffer(args.chunk) && args.chunk.length <= 500);
          state.data = Buffer.concat([state.data, args.chunk]);
          assert.ok(
            80 + state.data.length <= length,
            "append exceeds allocation",
          );
          this.actions.push({
            name,
            address: address.toBase58(),
            offset: state.data.length - args.chunk.length,
          });
        }
        const header = account.data.subarray(0, 80);
        account.data = Buffer.alloc(length);
        account.data.set(header);
        account.data.writeUInt32LE(state.data.length, 76);
        account.data.set(state.data, 80);
      } else if (name === "closeProofBuffer") {
        const account = await this.getAccountInfo(address);
        assert.ok(account);
        this.balance += account.lamports;
        this.accounts.delete(address.toBase58());
      } else if (name === "voidTransfer" || name === "mintTransfer") {
        const state = await program.account.proofBuffer.fetch(address);
        assert.equal(state.data.length, state.expectedLen);
        const proof = MerkleProof.decode(state.data),
          existence = proof.proofs[0].exist;
        assert.ok(existence);
        const id = object(args.id);
        assert.ok(
          Buffer.isBuffer(id.owner) &&
            id.height instanceof BN &&
            id.sequence instanceof BN,
        );
        assert.ok(args.proofHeight instanceof BN);
        const proofHeight = BigInt(String(args.proofHeight));
        const lc = await this.context.lightClient();
        const current = await lc.client.account.clientState.fetch(
          lc.clientState,
        );
        const consensusIndex = name === "voidTransfer" ? 6 : 7;
        assert.ok(
          keys[consensusIndex].equals(
            lc.consensusAt(
              BigInt(current.latestHeight.revisionHeight.toString()),
            ),
          ),
          "resolution must use the current latest consensus",
        );
        assert.ok(
          keys[consensusIndex + 1].equals(lc.consensusAt(proofHeight)),
          "resolution must use the retained proof consensus",
        );
        const status = keys[name === "voidTransfer" ? 3 : 4];
        assert.equal(
          await this.getAccountInfo(status),
          null,
          "outcome already exists",
        );
        const outcome =
          name === "voidTransfer"
            ? { voided: { reason: { governance: {} } } }
            : { minted: { amount: args.amount, recipient: args.recipientKey } };
        this.set(
          status,
          PROGRAM_ID,
          await program.coder.accounts.encode("transferStatus", {
            id,
            proofHeight: args.proofHeight,
            resolvedAtSlot: new BN(1),
            outcome,
          }),
        );
        this.balance -= 1_000_000;
        this.actions.push({
          name,
          address: address.toBase58(),
          key: Buffer.from(existence.key).toString("hex"),
        });
      } else throw new Error(`unexpected relay instruction ${name}`);
      if (
        name !== "appendProof" &&
        name !== "voidTransfer" &&
        name !== "mintTransfer"
      )
        this.actions.push({ name, address: address.toBase58() });
      this.afterAction?.(name);
    }
    this.transactions.set(signature, {
      slot: 1,
      blockTime: NOW,
      version: 0,
      transaction: { message: tx.message, signatures: [signature] },
      meta: {
        err: error,
        fee: 5000,
        preBalances: [before],
        postBalances: [this.balance],
        logMessages: [],
        computeUnitsConsumed: 1,
      },
    });
    if (this.crashAfter === action) {
      this.crashAfter = undefined;
      throw new Error("process lost RPC response after execution");
    }
    return signature;
  }
}

function proofOps(key: Uint8Array, value: Uint8Array, large: boolean) {
  const leaf = {
    hash: 1,
    prehashKey: 0,
    prehashValue: 1,
    length: 1,
    prefix: new Uint8Array([0]),
  };
  const iavl = CommitmentProof.fromPartial({
    exist: {
      key,
      value,
      leaf,
      path: large
        ? Array.from({ length: 85 }, () => ({
            hash: 1,
            prefix: new Uint8Array(64),
            suffix: new Uint8Array(64),
          }))
        : [],
    },
  });
  const store = CommitmentProof.fromPartial({
    exist: { key: Buffer.from("sink"), value: new Uint8Array(32), leaf },
  });
  return {
    ops: [
      { type: "ics23:iavl", key, data: CommitmentProof.encode(iavl).finish() },
      {
        type: "ics23:simple",
        key: Buffer.from("sink"),
        data: CommitmentProof.encode(store).finish(),
      },
    ],
  };
}

async function fixture(
  t: TestContext,
  status = Status.STATUS_VOIDED,
  large = false,
) {
  const stateDir = await temporaryDirectory("relay-recovery-");
  t.after(() => rm(stateDir, { recursive: true, force: true }));
  const connection = new PayingSolana(),
    wallet = new AnchorProvider(
      connection,
      new Wallet(web3.Keypair.generate()),
      { commitment: "finalized" },
    );
  const config = {
    ...relayTestConfig(stateDir),
    maxLamportsPerTransfer: 5_000_000,
  };
  let context = await relayContext({ config, wallet });
  connection.context = context;
  t.after(() => context.close());
  await testBridge(connection, context, true);
  let transfer = TransferState.fromPartial({
    ...testTransfer(1n, 50n),
    status,
    settlement: {
      round: 1n,
      policy: { version: 1n, members: ["reporter"], threshold: "1" },
      voidAuthorization: { requestedHeight: 60n, requestedRound: 1n },
    },
  });
  const id = parseSinkId(transfer.id);
  let corrupt: "key" | "value" | "height" | undefined;
  function connect() {
    context.akash.state = async () => ({
      ...QueryStateResponse.fromPartial({}),
      params: Params.fromPartial({}),
      policy: ReporterPolicy.fromPartial({}),
      sendUsage: EpochUsage.fromPartial({}),
      refundUsage: EpochUsage.fromPartial({}),
      totals: Totals.fromPartial({}),
      height: HEIGHT,
      time: BigInt(NOW),
    });
    context.akash.transfer = async (requested, height) => {
      assert.equal(requested, id);
      assert.ok(height <= HEIGHT);
      return TransferState.fromPartial(transfer);
    };
    context.akash.disputes = async () => ({
      transfers: [],
      nextKey: new Uint8Array(),
    });
    context.akash.transfers = async (_height, options = {}) => ({
      transfers:
        options.status === undefined || options.status === transfer.status
          ? [transfer]
          : [],
      nextKey: new Uint8Array(),
    });
    context.akash.comet.abciQuery = async (query) => {
      const key = transfer.settlement?.voidAuthorization
        ? settlementKey(id, transfer.status)
        : transferKey(id);
      const value = transfer.settlement?.voidAuthorization
        ? SettlementData.encode(transfer.settlement).finish()
        : encodeRecord(transfer.data);
      return {
        code: 0,
        codespace: "",
        log: "",
        info: "",
        index: 0,
        height: corrupt === "height" ? 1 : (query.height ?? 0),
        key: corrupt === "key" ? new Uint8Array([1]) : key,
        value: corrupt === "value" ? new Uint8Array([1]) : value,
        proof: proofOps(key, value, large),
      };
    };
  }
  connect();
  const logs: string[] = [];
  t.mock.method(console, "log", (line: string) => logs.push(line));
  t.mock.method(console, "error", (line: string) => logs.push(line));
  return {
    id,
    connection,
    logs,
    stateDir,
    get context() {
      return context;
    },
    get transfer() {
      return transfer;
    },
    set transfer(value: TransferState) {
      transfer = value;
    },
    corrupt(value: typeof corrupt) {
      corrupt = value;
    },
    async restart() {
      context.close();
      context = await relayContext({ config, wallet });
      connection.context = context;
      connect();
    },
    async ledger() {
      const file = (await readdir(stateDir)).find(
        (name) => name.startsWith("relay-budget-v2-") && name.endsWith(".json"),
      );
      assert.ok(file);
      return object(
        object(JSON.parse(await readFile(join(stateDir, file), "utf8")))
          .transfers,
      );
    },
  };
}

test("a real relayer tick grows, resumes a durable signed upload after restart, resolves and reclaims rent", async (t) => {
  const f = await fixture(t, Status.STATUS_VOIDED, true);
  f.connection.crashAfter = "appendProof";
  await f.context.tick();
  const first = f.connection.actions.filter(
    (action) => action.name === "appendProof",
  );
  assert.equal(first.length, 1, f.logs.join("\n"));
  assert.equal(first[0].offset, 0);
  const buffer = proofAddress(
    PROGRAM_ID,
    f.context.wallet.publicKey,
    settlementKey(f.id, Status.STATUS_VOIDED),
  );
  const allocated = await f.connection.getAccountInfo(buffer);
  assert.ok(allocated && allocated.data.length > 10240);
  const locked = allocated.lamports;
  assert.ok(
    locked > 5_000_000,
    "temporary rent exceeds the unchanged net-spend budget",
  );
  await f.restart();
  const advanced = Date.now() + 61_000;
  t.mock.method(Date, "now", () => advanced);
  await f.context.tick();
  assert.ok(!(await f.connection.getAccountInfo(buffer)), f.logs.join("\n"));
  const result = await f.context.program.account.transferStatus.fetch(
    statusAddress(PROGRAM_ID, f.id),
  );
  assert.ok(result.outcome.voided);
  assert.equal(
    f.connection.actions.filter((action) => action.name === "createProofBuffer")
      .length,
    1,
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "growProofBuffer")
      .length,
    1,
  );
  const offsets = f.connection.actions
    .filter((action) => action.name === "appendProof")
    .map((action) => action.offset);
  assert.deepEqual(
    offsets,
    offsets.map((_, i) => i * 500),
  );
  const entry = object((await f.ledger())[f.id]);
  assert.equal(entry.bufferReclaimedLamports, locked);
  assert.equal(entry.bufferFundedLamports, locked);
  assert.equal(
    entry.spentLamports,
    f.connection.transactions.size * 5000 + 1_000_000,
  );
  assert.equal(
    f.connection.balance,
    1_000_000_000 - Number(entry.spentLamports),
  );
  assert.ok(Number(entry.spentLamports) < 5_000_000);
  await f.context.tick();
  assert.equal(
    f.connection.actions.filter((action) => action.name === "voidTransfer")
      .length,
    1,
  );
});

test("a status and round change retires the exact old buffer and resumes the permanent order under a new key", async (t) => {
  const f = await fixture(t);
  f.connection.crashAfter = "createProofBuffer";
  await f.context.tick();
  const old = proofAddress(
    PROGRAM_ID,
    f.context.wallet.publicKey,
    settlementKey(f.id, Status.STATUS_VOIDED),
  );
  assert.ok(await f.connection.getAccountInfo(old), f.logs.join("\n"));
  f.transfer = TransferState.fromPartial({
    ...f.transfer,
    status: Status.STATUS_REFUNDED,
    settlement: {
      ...f.transfer.settlement,
      round: 3n,
      policy: { version: 2n, members: ["new"], threshold: "1" },
    },
  });
  await f.restart();
  const advanced = Date.now() + 61_000;
  t.mock.method(Date, "now", () => advanced);
  await f.context.tick();
  assert.equal(await f.connection.getAccountInfo(old), null);
  const resolution = f.connection.actions.find(
    (action) => action.name === "voidTransfer",
  );
  assert.equal(
    resolution?.key,
    Buffer.from(settlementKey(f.id, Status.STATUS_REFUNDED)).toString("hex"),
    f.logs.join("\n"),
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "mintTransfer")
      .length,
    0,
  );
});

for (const corruption of ["key", "value", "height"] as const)
  test(`a mismatched ${corruption} proof causes no paid transaction`, async (t) => {
    const f = await fixture(t);
    f.corrupt(corruption);
    await f.context.tick();
    assert.equal(f.connection.transactions.size, 0);
    assert.ok(f.logs.some((line) => line.includes("relay-retry")));
  });

for (const outcome of ["minted", "voided"] as const)
  test(`an actual ${outcome} PDA blocks every competing resolution`, async (t) => {
    const f = await fixture(t);
    assert.ok(f.transfer.data);
    f.connection.set(
      statusAddress(PROGRAM_ID, f.id),
      PROGRAM_ID,
      await f.context.program.coder.accounts.encode("transferStatus", {
        id: anchorId(f.id),
        proofHeight: new BN(TRUSTED),
        resolvedAtSlot: new BN(1),
        outcome:
          outcome === "minted"
            ? {
                minted: {
                  amount: new BN(60),
                  recipient: new web3.PublicKey(f.transfer.data.dest),
                },
              }
            : { voided: { reason: { governance: {} } } },
      }),
    );
    if (outcome === "voided")
      f.transfer = TransferState.fromPartial({
        ...f.transfer,
        status: Status.STATUS_TRANSFER_PENDING,
        settlement: { ...f.transfer.settlement, voidAuthorization: undefined },
      });
    await f.context.tick();
    assert.equal(f.connection.transactions.size, 0);
  });

for (const stage of ["growProofBuffer", "voidTransfer", "closeProofBuffer"])
  test(`restart reconciles a lost ${stage} response without duplicate resolution or rent loss`, async (t) => {
    const f = await fixture(
      t,
      Status.STATUS_REFUND_PENDING,
      stage === "growProofBuffer",
    );
    f.connection.crashAfter = stage;
    await f.context.tick();
    await f.restart();
    const advanced = Date.now() + 61_000;
    t.mock.method(Date, "now", () => advanced);
    await f.context.tick();
    assert.ok(
      await f.context.program.account.transferStatus.fetchNullable(
        statusAddress(PROGRAM_ID, f.id),
      ),
      f.logs.join("\n"),
    );
    const buffer = proofAddress(
      PROGRAM_ID,
      f.context.wallet.publicKey,
      settlementKey(f.id, Status.STATUS_REFUND_PENDING),
    );
    assert.equal(await f.connection.getAccountInfo(buffer), null);
    assert.equal(
      f.connection.actions.filter((action) => action.name === "voidTransfer")
        .length,
      1,
    );
    const entry = object((await f.ledger())[f.id]);
    assert.equal(entry.bufferFundedLamports, entry.bufferReclaimedLamports);
    assert.equal(
      entry.spentLamports,
      f.connection.transactions.size * 5000 + 1_000_000,
    );
  });

test("a settlement change during upload rechecks the round before resolution and closes its buffer", async (t) => {
  const f = await fixture(t);
  f.connection.afterAction = (name) => {
    if (name !== "appendProof") return;
    f.connection.afterAction = undefined;
    f.transfer = TransferState.fromPartial({
      ...f.transfer,
      settlement: { ...f.transfer.settlement, round: 2n },
    });
  };
  await f.context.tick();
  assert.equal(
    f.connection.actions.filter((action) => action.name === "voidTransfer")
      .length,
    0,
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "closeProofBuffer")
      .length,
    1,
  );
  await f.context.tick();
  assert.equal(
    f.connection.actions.filter((action) => action.name === "voidTransfer")
      .length,
    1,
  );
});

test("a retained proof journal resumes a terminal order after the spend ledger resets and history advances", async (t) => {
  const f = await fixture(t, Status.STATUS_REFUNDED);
  f.connection.crashAfter = "appendProof";
  await f.context.tick();
  const buffer = proofAddress(
    PROGRAM_ID,
    f.context.wallet.publicKey,
    settlementKey(f.id, Status.STATUS_REFUNDED),
  );
  assert.ok(await f.connection.getAccountInfo(buffer), f.logs.join("\n"));
  await f.context.resetBudget(f.id);
  assert.deepEqual(await f.ledger(), {});
  await f.restart();
  // The history cursor has already passed this terminal record. Only its
  // proof journal can put it back into the work queue after the reset.
  f.context.akash.transfers = async () => ({
    transfers: [],
    nextKey: new Uint8Array(),
  });
  await f.context.tick();
  assert.equal(await f.connection.getAccountInfo(buffer), null);
  assert.ok(
    (
      await f.context.program.account.transferStatus.fetch(
        statusAddress(PROGRAM_ID, f.id),
      )
    ).outcome.voided,
    f.logs.join("\n"),
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "createProofBuffer")
      .length,
    1,
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "voidTransfer")
      .length,
    1,
  );
});

test("a mint upload resumes against its retained proof while using the latest client consensus", async (t) => {
  const f = await fixture(t, Status.STATUS_TRANSFER_PENDING);
  f.transfer = TransferState.fromPartial({
    ...f.transfer,
    settlement: { ...f.transfer.settlement, voidAuthorization: undefined },
  });
  f.connection.crashAfter = "appendProof";
  await f.context.tick();
  assert.equal(
    f.connection.actions.filter((action) => action.name === "appendProof")
      .length,
    1,
    f.logs.join("\n"),
  );
  await f.restart();
  const lc = await f.context.lightClient();
  const latest = await lc.client.account.clientState.fetch(lc.clientState);
  latest.latestHeight.revisionHeight = new BN(TRUSTED + 1);
  f.connection.set(
    lc.clientState,
    lc.program,
    await lc.client.coder.accounts.encode("clientState", latest),
  );
  const consensus = await lc.client.account.consensusStateStore.fetch(
    lc.consensusAt(BigInt(TRUSTED)),
  );
  consensus.height = new BN(TRUSTED + 1);
  f.connection.set(
    lc.consensusAt(BigInt(TRUSTED + 1)),
    lc.program,
    await lc.client.coder.accounts.encode("consensusStateStore", consensus),
  );
  const advanced = Date.now() + 61_000;
  t.mock.method(Date, "now", () => advanced);
  await f.context.tick();
  assert.equal(
    f.connection.actions.filter((action) => action.name === "createProofBuffer")
      .length,
    1,
  );
  assert.equal(
    f.connection.actions.filter((action) => action.name === "mintTransfer")
      .length,
    1,
    f.logs.join("\n"),
  );
});

async function journals(f: Awaited<ReturnType<typeof fixture>>) {
  const directory = join(
    f.stateDir,
    "relay-jobs-v2",
    await f.connection.getGenesisHash(),
    PROGRAM_ID.toBase58(),
  );
  const files = await readdir(directory);
  const intent = files.find((name) => name.endsWith(".json"));
  assert.ok(intent);
  return {
    directory,
    intent: join(directory, intent),
    proof: join(directory, `${intent}.proof`),
  };
}

test("an expired resolution with missing history preserves its exact intent until accounting history returns", async (t) => {
  const f = await fixture(t);
  f.connection.crashAfter = "voidTransfer";
  await f.context.tick();
  const paths = await journals(f);
  const signed = await readFile(paths.intent, "utf8");
  const proof = await readFile(paths.proof, "utf8");
  const saved = object(JSON.parse(signed));
  assert.equal(saved.stage, "resolve");
  assert.ok(typeof saved.signature === "string");
  const transaction = f.connection.transactions.get(saved.signature);
  assert.ok(transaction);
  f.connection.transactions.delete(saved.signature);
  await f.restart();
  f.connection.getBlockHeight = async () => 2000;
  const latest = f.connection.getLatestBlockhash.bind(f.connection);
  f.connection.getLatestBlockhash = async () => ({
    ...(await latest()),
    lastValidBlockHeight: 3000,
  });
  const before = await f.ledger();
  await assert.rejects(
    () => f.context.tick(),
    /history unresolved.*intent retained/,
  );
  assert.equal(await readFile(paths.intent, "utf8"), signed);
  assert.equal(await readFile(paths.proof, "utf8"), proof);
  assert.deepEqual(await f.ledger(), before);
  assert.notEqual(object(before[f.id]).completed, true);
  assert.ok(
    await f.context.program.account.transferStatus.fetchNullable(
      statusAddress(PROGRAM_ID, f.id),
    ),
  );
  f.connection.transactions.set(saved.signature, transaction);
  await f.context.tick();
  const entry = object((await f.ledger())[f.id]);
  assert.equal(entry.completed, true);
  assert.equal(entry.spentLamports, 1_000_000_000 - f.connection.balance);
  assert.equal(entry.spentLamports, 1_020_000);
  assert.equal(entry.bufferFundedLamports, entry.bufferReclaimedLamports);
  assert.deepEqual(await readdir(paths.directory), []);
  const completed = await f.ledger();
  await f.context.tick();
  assert.deepEqual(await f.ledger(), completed);
  assert.equal(
    f.connection.actions.filter((action) => action.name === "voidTransfer")
      .length,
    1,
  );
});

async function prunedUpload(t: TestContext, kind: "mint" | "void") {
  const f = await fixture(
    t,
    kind === "mint" ? Status.STATUS_TRANSFER_PENDING : Status.STATUS_VOIDED,
  );
  if (kind === "mint")
    f.transfer = TransferState.fromPartial({
      ...f.transfer,
      settlement: { ...f.transfer.settlement, voidAuthorization: undefined },
    });
  f.connection.crashAfter = "appendProof";
  await f.context.tick();
  const paths = await journals(f);
  const key =
    kind === "mint"
      ? transferKey(f.id)
      : settlementKey(f.id, Status.STATUS_VOIDED);
  const buffer = proofAddress(PROGRAM_ID, f.context.wallet.publicKey, key);
  assert.ok(await f.connection.getAccountInfo(buffer));
  await f.restart();
  const lc = await f.context.lightClient();
  const latest = await lc.client.account.clientState.fetch(lc.clientState);
  latest.latestHeight.revisionHeight = new BN(TRUSTED + 1);
  f.connection.set(
    lc.clientState,
    lc.program,
    await lc.client.coder.accounts.encode("clientState", latest),
  );
  const consensus = await lc.client.account.consensusStateStore.fetch(
    lc.consensusAt(BigInt(TRUSTED)),
  );
  consensus.height = new BN(TRUSTED + 1);
  f.connection.set(
    lc.consensusAt(BigInt(TRUSTED + 1)),
    lc.program,
    await lc.client.coder.accounts.encode("consensusStateStore", consensus),
  );
  const state = f.context.akash.state.bind(f.context.akash);
  f.context.akash.state = async () => ({
    ...(await state()),
    height: HEIGHT + 1,
  });
  f.context.akash.transfer = async (id, height) => {
    assert.equal(id, f.id);
    if (height === TRUSTED - 1) throw new Error("Transfer query failed (38)");
    return TransferState.fromPartial(f.transfer);
  };
  const query = f.context.akash.comet.abciQuery.bind(f.context.akash.comet);
  f.context.akash.comet.abciQuery = async (request) => {
    if (request.height !== TRUSTED - 1) return query(request);
    assert.equal(request.path, "/store/sink/key");
    assert.equal(request.prove, false);
    assert.deepEqual(Buffer.from(request.data), Buffer.from(key));
    return {
      code: 0,
      codespace: "",
      log: "version does not exist",
      info: "",
      index: 0,
      height: TRUSTED - 1,
      key,
      value: new Uint8Array(),
    };
  };
  let now = Date.now() + 61_000;
  t.mock.method(Date, "now", () => now);
  return {
    f,
    paths,
    key,
    buffer,
    lc,
    query,
    advance: () => {
      now += 61_000;
    },
  };
}

for (const kind of ["mint", "void"] as const) {
  test(`a confirmed pruned ${kind} checkpoint retires only its buffer and resolves from a newer exact proof`, async (t) => {
    const { f, paths, key, buffer } = await prunedUpload(t, kind);
    const other = web3.Keypair.generate().publicKey;
    f.connection.set(other, PROGRAM_ID, Buffer.from("unrelated account"));
    const untouched = await f.connection.getAccountInfo(other);
    await f.context.tick();
    const status = await f.context.program.account.transferStatus.fetch(
      statusAddress(PROGRAM_ID, f.id),
    );
    assert.equal(status.proofHeight.toNumber(), TRUSTED + 1);
    assert.ok(kind === "mint" ? status.outcome.minted : status.outcome.voided);
    const resolutions = f.connection.actions.filter(
      (action) =>
        action.name === "mintTransfer" || action.name === "voidTransfer",
    );
    assert.equal(resolutions.length, 1, f.logs.join("\n"));
    assert.equal(resolutions[0].key, Buffer.from(key).toString("hex"));
    const closes = f.connection.actions.filter(
      (action) => action.name === "closeProofBuffer",
    );
    assert.deepEqual(
      closes.map((action) => action.address),
      [buffer.toBase58(), buffer.toBase58()],
    );
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "createProofBuffer",
      ).length,
      2,
    );
    assert.equal(await f.connection.getAccountInfo(buffer), null);
    assert.deepEqual(await f.connection.getAccountInfo(other), untouched);
    assert.deepEqual(await readdir(paths.directory), []);
    const entry = object((await f.ledger())[f.id]);
    assert.equal(entry.completed, true);
    assert.equal(entry.spentLamports, 1_000_000_000 - f.connection.balance);
    assert.equal(
      entry.spentLamports,
      f.connection.transactions.size * 5000 + 1_000_000,
    );
    assert.equal(entry.bufferFundedLamports, entry.bufferReclaimedLamports);
    await f.context.tick();
    assert.deepEqual(object((await f.ledger())[f.id]), entry);
  });

  test(`unknown ${kind} checkpoint failures preserve the proof job and rent`, async (t) => {
    const { f, paths, buffer, advance } = await prunedUpload(t, kind);
    const saved = await readFile(paths.proof, "utf8");
    const balance = f.connection.balance;
    const probe = f.context.akash.comet.abciQuery.bind(f.context.akash.comet);
    for (const fault of [
      "timeout",
      "unknown-code",
      "wrong-height",
      "wrong-key",
      "unexpected-value",
      "unclassified-log",
    ] as const) {
      f.context.akash.comet.abciQuery = async (request) => {
        if (fault === "timeout") throw new Error("Akash RPC unavailable");
        const response = await probe(request);
        switch (fault) {
          case "unknown-code":
            return { ...response, code: 38, codespace: "sdk" };
          case "wrong-height":
            return { ...response, height: TRUSTED - 2 };
          case "wrong-key":
            return { ...response, key: new Uint8Array([1]) };
          case "unexpected-value":
            return { ...response, value: new Uint8Array([1]) };
          case "unclassified-log":
            return { ...response, log: "state unavailable or pruned" };
        }
      };
      await f.context.tick();
      assert.equal(await readFile(paths.proof, "utf8"), saved, fault);
      assert.ok(await f.connection.getAccountInfo(buffer), fault);
      assert.equal(f.connection.balance, balance, fault);
      assert.notEqual(object((await f.ledger())[f.id]).completed, true, fault);
      advance();
    }
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "closeProofBuffer",
      ).length,
      0,
    );
    assert.equal(
      await f.context.program.account.transferStatus.fetchNullable(
        statusAddress(PROGRAM_ID, f.id),
      ),
      null,
    );
  });

  test(`a pruned ${kind} checkpoint cannot retire an upload with ambiguous signed history`, async (t) => {
    const { f, paths, buffer } = await prunedUpload(t, kind);
    const signed = await readFile(paths.intent, "utf8");
    const proof = await readFile(paths.proof, "utf8");
    const saved = object(JSON.parse(signed));
    assert.ok(typeof saved.signature === "string");
    const statuses = f.connection.getSignatureStatuses.bind(f.connection);
    f.connection.getSignatureStatuses = async () => {
      throw new Error("signature RPC unavailable");
    };
    await assert.rejects(() => f.context.tick(), /signature RPC unavailable/);
    assert.equal(await readFile(paths.intent, "utf8"), signed);
    assert.equal(await readFile(paths.proof, "utf8"), proof);
    f.connection.getSignatureStatuses = statuses;
    f.connection.transactions.delete(saved.signature);
    f.connection.getBlockHeight = async () => 2000;
    await assert.rejects(
      () => f.context.tick(),
      /history unresolved.*intent retained/,
    );
    assert.equal(await readFile(paths.intent, "utf8"), signed);
    assert.equal(await readFile(paths.proof, "utf8"), proof);
    assert.ok(await f.connection.getAccountInfo(buffer));
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "closeProofBuffer",
      ).length,
      0,
    );
    assert.notEqual(object((await f.ledger())[f.id]).completed, true);
  });
}

for (const kind of ["mint", "void"] as const) {
  test(`a pruned ${kind} upload waits for a newer checkpoint and a matching proof before retiring`, async (t) => {
    const { f, paths, buffer, lc, advance } = await prunedUpload(t, kind);
    const saved = await readFile(paths.proof, "utf8");
    const latest = await lc.client.account.clientState.fetch(lc.clientState);
    latest.latestHeight.revisionHeight = new BN(TRUSTED);
    f.connection.set(
      lc.clientState,
      lc.program,
      await lc.client.coder.accounts.encode("clientState", latest),
    );
    const balance = f.connection.balance;
    await f.context.tick();
    assert.equal(f.connection.balance, balance);
    assert.equal(await readFile(paths.proof, "utf8"), saved);
    assert.ok(await f.connection.getAccountInfo(buffer));
    latest.latestHeight.revisionHeight = new BN(TRUSTED + 1);
    f.connection.set(
      lc.clientState,
      lc.program,
      await lc.client.coder.accounts.encode("clientState", latest),
    );
    f.corrupt("value");
    advance();
    await f.context.tick();
    assert.equal(f.connection.balance, balance);
    assert.equal(await readFile(paths.proof, "utf8"), saved);
    assert.ok(await f.connection.getAccountInfo(buffer));
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "closeProofBuffer",
      ).length,
      0,
    );
    f.corrupt(undefined);
    advance();
    await f.context.tick();
    assert.equal(await f.connection.getAccountInfo(buffer), null);
    assert.equal(object((await f.ledger())[f.id]).completed, true);
  });

  test(`a lost ${kind} retirement response recovers its fee and rent before replacement`, async (t) => {
    const { f, paths, buffer, advance } = await prunedUpload(t, kind);
    f.connection.crashAfter = "closeProofBuffer";
    const saved = await readFile(paths.proof, "utf8");
    await f.context.tick();
    assert.equal(await f.connection.getAccountInfo(buffer), null);
    assert.equal(await readFile(paths.proof, "utf8"), saved);
    assert.equal(
      object(JSON.parse(await readFile(paths.intent, "utf8"))).stage,
      "proof-close",
    );
    advance();
    await f.context.tick();
    assert.deepEqual(await readdir(paths.directory), []);
    const entry = object((await f.ledger())[f.id]);
    assert.equal(entry.completed, true);
    assert.equal(entry.bufferFundedLamports, entry.bufferReclaimedLamports);
    assert.equal(entry.spentLamports, 1_000_000_000 - f.connection.balance);
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "closeProofBuffer",
      ).length,
      2,
    );
    assert.equal(
      f.connection.actions.filter(
        (action) => action.name === "createProofBuffer",
      ).length,
      2,
    );
  });
}

test("a landed failed append is charged once after restart before successful proof resumption", async (t) => {
  const f = await fixture(t);
  f.connection.failNext = "appendProof";
  f.connection.crashAfter = "appendProof";
  await f.context.tick();
  const paths = await journals(f);
  const saved = object(JSON.parse(await readFile(paths.intent, "utf8")));
  assert.ok(typeof saved.signature === "string");
  assert.ok(f.connection.transactions.get(saved.signature)?.meta?.err);
  await f.restart();
  const now = Date.now() + 61_000;
  t.mock.method(Date, "now", () => now);
  await f.context.tick();
  const entry = object((await f.ledger())[f.id]);
  assert.equal(entry.completed, true);
  assert.equal(entry.spentLamports, 1_000_000_000 - f.connection.balance);
  assert.equal(
    entry.spentLamports,
    f.connection.transactions.size * 5000 + 1_000_000,
  );
  assert.ok(Array.isArray(entry.signatures));
  assert.equal(
    entry.signatures.filter((signature) => signature === saved.signature)
      .length,
    1,
  );
  assert.equal(entry.bufferFundedLamports, entry.bufferReclaimedLamports);
  assert.deepEqual(await readdir(paths.directory), []);
  await f.context.tick();
  assert.deepEqual(object((await f.ledger())[f.id]), entry);
});
