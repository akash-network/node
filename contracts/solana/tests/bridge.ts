import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import type { TestContext } from "node:test";
import { BN, Program, web3 } from "@coral-xyz/anchor";
import {
  ASSOCIATED_TOKEN_PROGRAM_ID,
  AuthorityType,
  TOKEN_PROGRAM_ID,
  createAssociatedTokenAccountInstruction,
  createSetAuthorityInstruction,
  getAccount,
  getAssociatedTokenAddressSync,
  getMint,
} from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { AktSolana } from "../target/types/akt_solana";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import { SolanaObserver, decodeRecord } from "../reporter/solana";
import { transferKey, voidKey } from "../shared/sink";
import { Outcome } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import {
  emittedEvent,
  rejectsWithAnchorCode,
  rejectsWithProgramLogs,
  testProvider,
} from "./helpers";
import { FROZEN_CLIENT, REPIN_CLIENT } from "./relocate-program";

const LOADER = new web3.PublicKey(
  "BPFLoaderUpgradeab1e11111111111111111111111",
);
const HEIGHT = new BN(42);
const CHAIN = "akash-local-1";
// Upstream's test-cpi-proxy at the pinned revision; scripts/test-contract.sh loads it.
const CPI_PROXY = new web3.PublicKey(
  "CtQLLKbDMt1XVNXtLKJEt1K8cstbckjqE6zyFqR37KTc",
);

function le64(value: BN) {
  return value.toArrayLike(Buffer, "le", 8);
}
// Fixture entries and proofs are keyed by the hex sink store key.
function hexKey(key: Uint8Array) {
  return Buffer.from(key).toString("hex");
}
function makeRecord(id: number, recipient: web3.PublicKey, amount: number) {
  return {
    version: 1,
    id: new BN(id),
    sender: Array<number>(20).fill(42),
    recipient,
    amount: new BN(amount),
    height: new BN(41),
  };
}
function u32(value: number) {
  const bytes = Buffer.alloc(4);
  bytes.writeUInt32LE(value);
  return bytes;
}
// Wraps an instruction in proxy_cpi(instruction_data, account_metas).
function viaProxy(inner: web3.TransactionInstruction, payer: web3.PublicKey) {
  return new web3.TransactionInstruction({
    programId: CPI_PROXY,
    keys: [
      { pubkey: inner.programId, isSigner: false, isWritable: false },
      { pubkey: payer, isSigner: true, isWritable: true },
      ...inner.keys,
    ],
    data: Buffer.concat([
      createHash("sha256").update("global:proxy_cpi").digest().subarray(0, 8),
      u32(inner.data.length),
      inner.data,
      u32(inner.keys.length),
      ...inner.keys.map((key) =>
        Buffer.from([Number(key.isSigner), Number(key.isWritable)]),
      ),
    ]),
  });
}
function encodeRecord(record: ReturnType<typeof makeRecord>) {
  return Buffer.concat([
    Buffer.from([record.version]),
    le64(record.id),
    Buffer.from(record.sender),
    record.recipient.toBuffer(),
    le64(record.amount),
    le64(record.height),
  ]);
}

export async function runBridgeTests(t: TestContext) {
  const provider = testProvider();
  const program = new Program<AktSolana>(idl, provider);
  const client = new Program<Ics07Tendermint>(clientIdl, provider);
  const access = new Program<AccessManager>(accessIdl, provider);
  const connection = provider.connection;
  const pda = (seed: string, owner = program.programId) =>
    web3.PublicKey.findProgramAddressSync([Buffer.from(seed)], owner)[0];
  const programData = (owner: web3.PublicKey) =>
    web3.PublicKey.findProgramAddressSync([owner.toBuffer()], LOADER)[0];
  const config = pda("config");
  const bridge = pda("bridge");
  const mint = pda("mint");
  const mintAuthority = pda("mint_authority");
  const clientState = pda("client", client.programId);
  const consensus = web3.PublicKey.findProgramAddressSync(
    [Buffer.from("consensus_state"), le64(HEIGHT)],
    client.programId,
  )[0];
  const recipient = web3.Keypair.generate();
  const outsider = web3.Keypair.generate();
  const reassigned = web3.Keypair.generate();
  const vault = pda("test-vault", access.programId);
  const unlistedVault = pda("unlisted-vault", access.programId);
  const records = [
    makeRecord(1, recipient.publicKey, 250_000_000),
    makeRecord(2, recipient.publicKey, 700_000_000),
    makeRecord(3, mint, 10_000_000),
    makeRecord(4, recipient.publicKey, 20_000_000),
    makeRecord(5, recipient.publicKey, 100_000_000),
    makeRecord(6, recipient.publicKey, 10_000_000),
    makeRecord(7, vault, 10_000_000),
    makeRecord(8, unlistedVault, 10_000_000),
    makeRecord(9, recipient.publicKey, 10_000_000),
    makeRecord(10, reassigned.publicKey, 10_000_000),
    // An executable recipient and the zero key.
    makeRecord(11, TOKEN_PROGRAM_ID, 10_000_000),
    makeRecord(12, web3.PublicKey.default, 10_000_000),
  ];
  function record(id: number) {
    const value = records.find((value) => value.id.eqn(id));
    assert.ok(value);
    return value;
  }
  const input = [
    ...records.map((value) => ({
      key: hexKey(transferKey(value.id.toNumber())),
      value: encodeRecord(value).toString("hex"),
    })),
    ...[1, 4].map((id) => ({
      key: hexKey(voidKey(id)),
      value: Buffer.concat([Buffer.from([1]), le64(new BN(id))]).toString(
        "hex",
      ),
    })),
  ];
  const fixture: unknown = JSON.parse(
    execFileSync("go", ["run", "."], {
      cwd: "tools/proof-fixtures",
      input: JSON.stringify(input),
      encoding: "utf8",
    }),
  );
  assert.ok(
    typeof fixture === "object" &&
      fixture !== null &&
      "root" in fixture &&
      typeof fixture.root === "string" &&
      "entries" in fixture &&
      Array.isArray(fixture.entries),
  );
  const root = Buffer.from(fixture.root, "hex");
  assert.equal(root.length, 32);
  const proofs = new Map<string, Buffer>();
  fixture.entries.forEach((entry: unknown) => {
    assert.ok(
      typeof entry === "object" &&
        entry !== null &&
        "key" in entry &&
        typeof entry.key === "string" &&
        "proof" in entry &&
        typeof entry.proof === "string",
    );
    proofs.set(entry.key, Buffer.from(entry.proof, "hex"));
  });
  function proof(key: Uint8Array) {
    const value = proofs.get(hexKey(key));
    assert.ok(value);
    return value;
  }

  await provider.sendAndConfirm(
    new web3.Transaction().add(
      web3.SystemProgram.transfer({
        fromPubkey: provider.publicKey,
        toPubkey: outsider.publicKey,
        lamports: web3.LAMPORTS_PER_SOL,
      }),
    ),
  );
  const accessManager = pda("access_manager", access.programId);
  await access.methods
    .initialize(provider.publicKey)
    .accountsStrict({
      accessManager,
      payer: provider.publicKey,
      systemProgram: web3.SystemProgram.programId,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
      programData: programData(access.programId),
      authority: provider.publicKey,
    })
    .rpc();
  await access.methods
    .grantRole(new BN(1), provider.publicKey)
    .accountsStrict({
      accessManager,
      admin: provider.publicKey,
      instructionsSysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
    })
    .rpc();

  // This is a deliberately chosen local checkpoint, not an authenticated Akash block.
  const timestamp = Math.floor(Date.now() / 1000);
  const trustingPeriod = 60;
  const latestConsensus = (owner: web3.PublicKey) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from("consensus_state"), le64(HEIGHT)],
      owner,
    )[0];
  async function initializeClient(
    address: web3.PublicKey,
    trusting: number,
    frozen = false,
  ) {
    const verifier = new Program<Ics07Tendermint>(
      { ...clientIdl, address: address.toBase58() },
      provider,
    );
    await verifier.methods
      .initialize(
        {
          chainId: CHAIN,
          trustLevelNumerator: new BN(1),
          trustLevelDenominator: new BN(3),
          trustingPeriod: new BN(trusting),
          unbondingPeriod: new BN(trusting * 2),
          maxClockDrift: new BN(5),
          frozenHeight: frozen
            ? { revisionNumber: new BN(1), revisionHeight: HEIGHT }
            : { revisionNumber: new BN(0), revisionHeight: new BN(0) },
          latestHeight: { revisionNumber: new BN(1), revisionHeight: HEIGHT },
        },
        {
          timestamp: new BN(timestamp).mul(new BN(1_000_000_000)),
          root: [...root],
          nextValidatorsHash: Array<number>(32).fill(7),
        },
        access.programId,
      )
      .accountsStrict({
        clientStateAccount: pda("client", address),
        consensusStateStore: latestConsensus(address),
        appState: pda("app_state", address),
        payer: provider.publicKey,
        systemProgram: web3.SystemProgram.programId,
        programData: programData(address),
        authority: provider.publicKey,
      })
      .rpc();
  }
  await initializeClient(client.programId, trustingPeriod);
  // Other deployments of the same verifier, for re-pinning. The second is
  // frozen from the start.
  await initializeClient(REPIN_CLIENT, 3_600);
  await initializeClient(FROZEN_CLIENT, 3_600, true);
  const replacementClient = pda("client", REPIN_CLIENT);
  const replacementConsensus = latestConsensus(REPIN_CLIENT);

  const admin = {
    program: program.programId,
    programData: programData(program.programId),
    authority: provider.publicKey,
  };
  const policy = {
    perTransferMax: new BN(600_000_000),
    dailyMintCap: new BN(1_000_000_000),
    vaults: [vault],
  };
  const configuration = {
    admin,
    config,
    bridge,
    lightClientProgram: client.programId,
    clientState,
    latestConsensus: consensus,
    systemProgram: web3.SystemProgram.programId,
  };
  const management = { admin, config, bridge };
  // The pinned client's accounts are not inputs.
  const repinAccounts = (address: web3.PublicKey) => ({
    admin,
    bridge,
    lightClientProgram: address,
    clientState: pda("client", address),
    latestConsensus: latestConsensus(address),
  });
  const repinning = repinAccounts(REPIN_CLIENT);
  async function repin(address: web3.PublicKey) {
    const signature = await program.methods
      .repinLightClient(CHAIN)
      .accountsStrict(repinAccounts(address))
      .rpc();
    return emittedEvent(program, signature, "lightClientRepinned");
  }
  // Decoded u64 fields are BN and keys are PublicKey; both serialize to JSON
  // as their canonical strings.
  const assertSamePolicy = (actual: unknown, expected: typeof policy) =>
    assert.equal(JSON.stringify(actual), JSON.stringify(expected));

  await t.test(
    "rejects unauthorized configuration and a wrong chain",
    async () => {
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .configureBridge(CHAIN, policy)
            .accountsStrict({
              ...configuration,
              admin: { ...admin, authority: outsider.publicKey },
            })
            .signers([outsider])
            .rpc(),
        "InvalidAuthority",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .configureBridge("other-chain", policy)
            .accountsStrict(configuration)
            .rpc(),
        "WrongChain",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .configureBridge("", policy)
            .accountsStrict(configuration)
            .rpc(),
        "InvalidChainId",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .configureBridge(CHAIN, { ...policy, perTransferMax: new BN(0) })
            .accountsStrict(configuration)
            .rpc(),
        "InvalidPolicy",
      );
      assert.equal(await connection.getAccountInfo(bridge), null);
    },
  );
  await t.test(
    "pins the real client once and keeps minting disabled",
    async () => {
      const configured = await emittedEvent(
        program,
        await program.methods
          .configureBridge(CHAIN, policy)
          .accountsStrict(configuration)
          .rpc(),
        "bridgeConfigured",
      );
      assert.ok(configured.lightClientProgram.equals(client.programId));
      assert.equal(configured.chainId, CHAIN);
      assertSamePolicy(configured.policy, policy);
      assert.equal(
        (await program.account.launchConfig.fetch(config)).bridgeEnabled,
        false,
      );
      assert.ok(
        (
          await program.account.bridgeConfig.fetch(bridge)
        ).lightClientProgram.equals(client.programId),
      );
      await rejectsWithProgramLogs(
        () =>
          program.methods
            .configureBridge(CHAIN, policy)
            .accountsStrict(configuration)
            .preInstructions([
              web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 210_000 }),
            ])
            .rpc(),
        /already in use/,
      );
    },
  );

  let nonce = 0;
  async function createBuffer(data: Buffer) {
    const next = new BN(++nonce);
    const address = web3.PublicKey.findProgramAddressSync(
      [Buffer.from("proof"), provider.publicKey.toBuffer(), le64(next)],
      program.programId,
    )[0];
    await program.methods
      .createProofBuffer(next, data.length)
      .accountsStrict({
        submitter: provider.publicKey,
        proofBuffer: address,
        systemProgram: web3.SystemProgram.programId,
      })
      .rpc();
    return address;
  }
  async function upload(data: Buffer) {
    const address = await createBuffer(data);
    for (let offset = 0; offset < data.length; offset += 500) {
      await program.methods
        .appendProof(offset, data.subarray(offset, offset + 500))
        .accountsStrict({ submitter: provider.publicKey, proofBuffer: address })
        .rpc();
    }
    return address;
  }
  const proofBuffers = new Map<string, web3.PublicKey>();
  async function buffer(key: Uint8Array) {
    const existing = proofBuffers.get(hexKey(key));
    if (existing) return existing;
    const address = await upload(proof(key));
    proofBuffers.set(hexKey(key), address);
    return address;
  }
  const status = (id: BN) =>
    web3.PublicKey.findProgramAddressSync(
      [Buffer.from("transfer"), le64(id)],
      program.programId,
    )[0];
  function mintAccounts(
    value: ReturnType<typeof makeRecord>,
    proofBuffer: web3.PublicKey,
  ) {
    return {
      payer: provider.publicKey,
      config,
      bridge,
      proofBuffer,
      status: status(value.id),
      lightClientProgram: client.programId,
      clientState,
      latestConsensus: consensus,
      proofConsensus: consensus,
      mint,
      mintAuthority,
      recipient: value.recipient,
      recipientTokenAccount: getAssociatedTokenAddressSync(
        mint,
        value.recipient,
        true,
      ),
      tokenProgram: TOKEN_PROGRAM_ID,
      associatedTokenProgram: ASSOCIATED_TOKEN_PROGRAM_ID,
      systemProgram: web3.SystemProgram.programId,
    };
  }
  const compute = () => [
    web3.ComputeBudgetProgram.setComputeUnitLimit({ units: 1_400_000 }),
    web3.ComputeBudgetProgram.requestHeapFrame({ bytes: 256 * 1024 }),
  ];
  async function mintRecord(id: number) {
    const value = record(id);
    return program.methods
      .mintTransfer(value, HEIGHT)
      .accountsStrict(mintAccounts(value, await buffer(transferKey(id))))
      .preInstructions(compute())
      .rpc();
  }
  async function voidAccounts(id: number) {
    return {
      payer: provider.publicKey,
      bridge,
      proofBuffer: await buffer(voidKey(id)),
      status: status(new BN(id)),
      lightClientProgram: client.programId,
      clientState,
      latestConsensus: consensus,
      proofConsensus: consensus,
      systemProgram: web3.SystemProgram.programId,
    };
  }
  async function voidRecord(id: number) {
    return program.methods
      .voidTransfer(new BN(id), HEIGHT)
      .accountsStrict(await voidAccounts(id))
      .preInstructions(compute())
      .rpc();
  }
  async function assertUnresolved(id: number) {
    assert.equal(await connection.getAccountInfo(status(new BN(id))), null);
  }

  const incomplete = await createBuffer(proof(transferKey(1)));
  await t.test(
    "protects proof uploads from other writers, gaps and oversize chunks",
    async () => {
      for (const length of [0, 4097]) {
        const next = new BN(++nonce);
        await rejectsWithAnchorCode(
          () =>
            program.methods
              .createProofBuffer(next, length)
              .accountsStrict({
                submitter: provider.publicKey,
                proofBuffer: web3.PublicKey.findProgramAddressSync(
                  [
                    Buffer.from("proof"),
                    provider.publicKey.toBuffer(),
                    le64(next),
                  ],
                  program.programId,
                )[0],
                systemProgram: web3.SystemProgram.programId,
              })
              .rpc(),
          "InvalidProofLength",
        );
      }
      assert.ok(proof(transferKey(1)).length > 500);
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .appendProof(0, Buffer.from([1]))
            .accountsStrict({
              submitter: outsider.publicKey,
              proofBuffer: incomplete,
            })
            .signers([outsider])
            .rpc(),
        "ConstraintHasOne",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .appendProof(1, Buffer.from([1]))
            .accountsStrict({
              submitter: provider.publicKey,
              proofBuffer: incomplete,
            })
            .rpc(),
        "InvalidProofOffset",
      );
      const small = await createBuffer(Buffer.alloc(1));
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .appendProof(0, Buffer.from([1, 2]))
            .accountsStrict({
              submitter: provider.publicKey,
              proofBuffer: small,
            })
            .rpc(),
        "InvalidProofLength",
      );
      await program.methods
        .closeProofBuffer()
        .accountsStrict({ submitter: provider.publicKey, proofBuffer: small })
        .rpc();
    },
  );
  await t.test(
    "rejects disabled bridge issuance and unauthorized activation",
    async () => {
      await rejectsWithAnchorCode(() => mintRecord(1), "BridgeDisabled");
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .setBridgeEnabled(true)
            .accountsStrict({
              ...management,
              admin: { ...admin, authority: outsider.publicKey },
            })
            .signers([outsider])
            .rpc(),
        "InvalidAuthority",
      );
      await assertUnresolved(1);
      const enabled = await emittedEvent(
        program,
        await program.methods
          .setBridgeEnabled(true)
          .accountsStrict(management)
          .rpc(),
        "bridgeEnabledSet",
      );
      assert.equal(enabled.enabled, true);
      assert.ok(enabled.authority.equals(provider.publicKey));
    },
  );
  await t.test(
    "lets the pause guardian disable but never enable minting",
    async () => {
      const guardian = web3.Keypair.generate();
      const pauseGuardian = pda("guardian");
      const setGuardian = (key: web3.PublicKey | null, authority = admin) =>
        program.methods.setPauseGuardian(key).accountsStrict({
          admin: authority,
          pauseGuardian,
          systemProgram: web3.SystemProgram.programId,
        });
      const pause = (signer: web3.Keypair) =>
        program.methods
          .guardianPause()
          .accountsStrict({ guardian: signer.publicKey, pauseGuardian, config })
          .signers([signer])
          .rpc();
      const enabled = async () =>
        (await program.account.launchConfig.fetch(config)).bridgeEnabled;

      await rejectsWithAnchorCode(
        () =>
          setGuardian(guardian.publicKey, {
            ...admin,
            authority: outsider.publicKey,
          })
            .signers([outsider])
            .rpc(),
        "InvalidAuthority",
      );
      const named = await emittedEvent(
        program,
        await setGuardian(guardian.publicKey).rpc(),
        "pauseGuardianSet",
      );
      assert.equal(named.previous, null);
      assert.ok(named.guardian.equals(guardian.publicKey));
      await rejectsWithAnchorCode(
        () => pause(outsider),
        "UnauthorizedGuardian",
      );
      assert.equal(await enabled(), true);
      const paused = await emittedEvent(
        program,
        await pause(guardian),
        "guardianPaused",
      );
      assert.ok(paused.guardian.equals(guardian.publicKey));
      assert.equal(await enabled(), false);
      await rejectsWithAnchorCode(() => mintRecord(1), "BridgeDisabled");
      // Enabling stays with the upgrade authority.
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .setBridgeEnabled(true)
            .accountsStrict({
              ...management,
              admin: { ...admin, authority: guardian.publicKey },
            })
            .signers([guardian])
            .rpc(),
        "InvalidAuthority",
      );
      assert.equal(await enabled(), false);

      const rotated = await emittedEvent(
        program,
        await setGuardian(outsider.publicKey).rpc(),
        "pauseGuardianSet",
      );
      assert.ok(rotated.previous.equals(guardian.publicKey));
      assert.ok(rotated.guardian.equals(outsider.publicKey));
      await program.methods
        .setBridgeEnabled(true)
        .accountsStrict(management)
        .rpc();
      await rejectsWithAnchorCode(
        () => pause(guardian),
        "UnauthorizedGuardian",
      );
      await pause(outsider);
      assert.equal(await enabled(), false);
      const cleared = await emittedEvent(
        program,
        await setGuardian(null).rpc(),
        "pauseGuardianSet",
      );
      assert.ok(cleared.previous.equals(outsider.publicKey));
      assert.equal(cleared.guardian, null);
      assert.equal(
        (await program.account.pauseGuardian.fetch(pauseGuardian)).guardian,
        null,
      );
      await rejectsWithAnchorCode(
        () => pause(outsider),
        "UnauthorizedGuardian",
      );
      await program.methods
        .setBridgeEnabled(true)
        .accountsStrict(management)
        .rpc();
      await assertUnresolved(1);
    },
  );
  await t.test(
    "re-pins an active client only to a checked deployment",
    async () => {
      assert.deepEqual(
        idl.instructions
          .find((instruction) => instruction.name === "repin_light_client")
          ?.accounts.map((account) => account.name),
        [
          "admin",
          "bridge",
          "light_client_program",
          "client_state",
          "latest_consensus",
        ],
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .repinLightClient(CHAIN)
            .accountsStrict({
              ...repinning,
              admin: { ...admin, authority: outsider.publicKey },
            })
            .signers([outsider])
            .rpc(),
        "InvalidAuthority",
      );
      // The replacement gets the same checks as configure_bridge.
      const rejected: [string, ReturnType<typeof repinAccounts>, string][] = [
        ["other-chain", repinning, "WrongChain"],
        ["", repinning, "InvalidChainId"],
        ["a".repeat(65), repinning, "InvalidChainId"],
        [
          CHAIN,
          { ...repinning, lightClientProgram: outsider.publicKey },
          "ConstraintExecutable",
        ],
        [CHAIN, repinAccounts(access.programId), "InvalidLightClient"],
        [CHAIN, { ...repinning, clientState }, "InvalidLightClient"],
        [
          CHAIN,
          { ...repinning, latestConsensus: consensus },
          "InvalidLightClient",
        ],
        [CHAIN, repinAccounts(FROZEN_CLIENT), "ClientFrozen"],
      ];
      for (const [chainId, accounts, code] of rejected) {
        await rejectsWithAnchorCode(
          () =>
            program.methods
              .repinLightClient(chainId)
              .accountsStrict(accounts)
              .rpc(),
          code,
        );
      }
      const before = await program.account.bridgeConfig.fetch(bridge);
      assert.ok(before.lightClientProgram.equals(client.programId));

      const moved = await repin(REPIN_CLIENT);
      assert.ok(moved.previousProgram.equals(client.programId));
      assert.ok(moved.program.equals(REPIN_CLIENT));
      assert.equal(moved.previousChainId, CHAIN);
      assert.equal(moved.chainId, CHAIN);
      const after = await program.account.bridgeConfig.fetch(bridge);
      assert.ok(after.lightClientProgram.equals(REPIN_CLIENT));
      assert.equal(JSON.stringify(after.policy), JSON.stringify(before.policy));
      assert.equal(after.mintDay.toString(), before.mintDay.toString());
      assert.equal(after.mintedToday.toString(), before.mintedToday.toString());
      // The previous deployment is refused at once, though still active.
      await rejectsWithAnchorCode(() => mintRecord(1), "ConstraintAddress");
      await assertUnresolved(1);

      const restored = await repin(client.programId);
      assert.ok(restored.previousProgram.equals(REPIN_CLIENT));
      assert.ok(restored.program.equals(client.programId));
      assert.ok(
        (
          await program.account.bridgeConfig.fetch(bridge)
        ).lightClientProgram.equals(client.programId),
      );
    },
  );
  await t.test(
    "rejects incomplete proofs and untrusted client accounts",
    async () => {
      const accounts = mintAccounts(record(1), await buffer(transferKey(1)));
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT)
            .accountsStrict({ ...accounts, proofBuffer: incomplete })
            .preInstructions(compute())
            .rpc(),
        "IncompleteProof",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT)
            .accountsStrict({
              ...accounts,
              lightClientProgram: access.programId,
            })
            .preInstructions(compute())
            .rpc(),
        "ConstraintAddress",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT)
            .accountsStrict({ ...accounts, proofConsensus: clientState })
            .preInstructions(compute())
            .rpc(),
        "InvalidLightClient",
      );
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT.addn(1))
            .accountsStrict(accounts)
            .preInstructions(compute())
            .rpc(),
        "InvalidProofHeight",
      );
      await assertUnresolved(1);
    },
  );
  await t.test(
    "rejects modified amounts, recipients, IDs and proof bytes in the real verifier",
    async () => {
      const firstProof = await buffer(transferKey(1));
      for (const changed of [
        { ...record(1), amount: new BN(251_000_000) },
        { ...record(1), recipient: outsider.publicKey },
        { ...record(1), id: new BN(99) },
      ]) {
        await rejectsWithProgramLogs(
          () =>
            program.methods
              .mintTransfer(changed, HEIGHT)
              .accountsStrict(mintAccounts(changed, firstProof))
              .preInstructions(compute())
              .rpc(),
          /MembershipVerificationFailed/,
        );
      }
      const damaged = Buffer.from(proof(transferKey(1)));
      damaged[damaged.length - 1] ^= 1;
      const damagedBuffer = await upload(damaged);
      await rejectsWithProgramLogs(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT)
            .accountsStrict(mintAccounts(record(1), damagedBuffer))
            .preInstructions(compute())
            .rpc(),
        /MembershipVerificationFailed/,
      );
      await assertUnresolved(1);
      assert.equal((await getMint(connection, mint)).supply, 0n);
    },
  );
  await t.test(
    "mints to the derived ATA only after upstream proof verification",
    async () => {
      const signature = await mintRecord(1);
      const resolved = await emittedEvent(
        program,
        signature,
        "transferResolved",
      );
      assert.ok(resolved.id.eqn(1));
      assert.ok(resolved.proofHeight.eq(HEIGHT));
      assert.equal(resolved.outcome.minted.amount.toString(), "250000000");
      assert.ok(resolved.outcome.minted.recipient.equals(recipient.publicKey));
      const transaction = await connection.getTransaction(signature, {
        commitment: "confirmed",
        maxSupportedTransactionVersion: 0,
      });
      const metadata = transaction?.meta;
      assert.ok(metadata);
      assert.ok(
        metadata.logMessages?.some((line) =>
          line.includes("Instruction: VerifyMembership"),
        ),
      );
      t.diagnostic(
        `Verified mint: ${proof(transferKey(1)).length}-byte ICS23 proof, ${metadata.computeUnitsConsumed} compute units including ATA creation.`,
      );
      assert.equal((await getMint(connection, mint)).supply, 250_000_000n);
      assert.equal(
        (
          await getAccount(
            connection,
            getAssociatedTokenAddressSync(mint, recipient.publicKey),
          )
        ).amount,
        250_000_000n,
      );
      assert.ok(
        (await program.account.transferStatus.fetch(status(new BN(1)))).outcome
          .minted,
      );
    },
  );
  await t.test(
    "rejects replay and a governance void after minting",
    async () => {
      // A different compute budget gives replay a distinct transaction signature.
      const value = record(1);
      const firstProof = await buffer(transferKey(1));
      await rejectsWithProgramLogs(
        () =>
          program.methods
            .mintTransfer(value, HEIGHT)
            .accountsStrict(mintAccounts(value, firstProof))
            .preInstructions([
              web3.ComputeBudgetProgram.setComputeUnitLimit({
                units: 1_300_000,
              }),
            ])
            .rpc(),
        /already in use/,
      );
      await rejectsWithProgramLogs(() => voidRecord(1), /already in use/);
      assert.equal((await getMint(connection, mint)).supply, 250_000_000n);
    },
  );
  await t.test(
    "voids an invalid recipient without creating its ATA or minting",
    async () => {
      // A token account, an executable and the zero key.
      for (const id of [3, 11, 12]) {
        await mintRecord(id);
        assert.ok(
          (await program.account.transferStatus.fetch(status(new BN(id))))
            .outcome.voided?.reason.invalidRecipient,
        );
        assert.equal(
          await connection.getAccountInfo(
            getAssociatedTokenAddressSync(mint, record(id).recipient, true),
          ),
          null,
        );
      }
      assert.equal((await getMint(connection, mint)).supply, 250_000_000n);
    },
  );
  await t.test("voids a recipient whose ATA owner was reassigned", async () => {
    // Legacy SPL Token lets the holder hand its ATA to another owner.
    // create_idempotent would then fail on every retry.
    const ata = getAssociatedTokenAddressSync(mint, reassigned.publicKey);
    await provider.sendAndConfirm(
      new web3.Transaction().add(
        createAssociatedTokenAccountInstruction(
          provider.publicKey,
          ata,
          reassigned.publicKey,
          mint,
        ),
        createSetAuthorityInstruction(
          ata,
          reassigned.publicKey,
          AuthorityType.AccountOwner,
          outsider.publicKey,
        ),
      ),
      [reassigned],
    );
    await mintRecord(10);
    assert.ok(
      (await program.account.transferStatus.fetch(status(new BN(10)))).outcome
        .voided?.reason.invalidRecipient,
    );
    const account = await getAccount(connection, ata);
    assert.ok(account.owner.equals(outsider.publicKey));
    assert.equal(account.amount, 0n);
    assert.equal((await getMint(connection, mint)).supply, 250_000_000n);
  });
  await t.test(
    "rejects mint and void called through another program",
    async () => {
      // Anchor's event parser ignores events emitted inside a CPI, so reporters
      // would never see these resolutions. Both proofs are valid top-level.
      const inner = [
        await program.methods
          .mintTransfer(record(6), HEIGHT)
          .accountsStrict(mintAccounts(record(6), await buffer(transferKey(6))))
          .instruction(),
        await program.methods
          .voidTransfer(new BN(4), HEIGHT)
          .accountsStrict(await voidAccounts(4))
          .instruction(),
      ];
      for (const instruction of inner) {
        await rejectsWithProgramLogs(
          () =>
            provider.sendAndConfirm(
              new web3.Transaction().add(
                ...compute(),
                viaProxy(instruction, provider.publicKey),
              ),
            ),
          new RegExp(
            `Program ${CPI_PROXY.toBase58()} invoke \\[1\\][\\s\\S]*` +
              `Program ${program.programId.toBase58()} invoke \\[2\\]` +
              `[\\s\\S]*Error Code: CpiNotAllowed`,
          ),
        );
      }
      await assertUnresolved(6);
      await assertUnresolved(4);
      assert.equal((await getMint(connection, mint)).supply, 250_000_000n);
    },
  );
  await t.test(
    "rejects mint and void after an instruction other than compute budget",
    async () => {
      // Earlier instructions' logs could fill the 10 KB log limit and
      // truncate the TransferResolved event.
      const inner = [
        await program.methods
          .mintTransfer(record(6), HEIGHT)
          .accountsStrict(mintAccounts(record(6), await buffer(transferKey(6))))
          .instruction(),
        await program.methods
          .voidTransfer(new BN(4), HEIGHT)
          .accountsStrict(await voidAccounts(4))
          .instruction(),
      ];
      for (const instruction of inner) {
        await rejectsWithProgramLogs(
          () =>
            provider.sendAndConfirm(
              new web3.Transaction().add(
                ...compute(),
                web3.SystemProgram.transfer({
                  fromPubkey: provider.publicKey,
                  toPubkey: outsider.publicKey,
                  lamports: 1,
                }),
                instruction,
              ),
            ),
          /Error Code: PrecedingInstruction/,
        );
      }
      await assertUnresolved(6);
      await assertUnresolved(4);
    },
  );
  await t.test(
    "honors a proven governance void while minting is paused",
    async () => {
      const disabled = await emittedEvent(
        program,
        await program.methods
          .setBridgeEnabled(false)
          .accountsStrict(management)
          .rpc(),
        "bridgeEnabledSet",
      );
      assert.equal(disabled.enabled, false);
      const voided = await emittedEvent(
        program,
        await voidRecord(4),
        "transferResolved",
      );
      assert.ok(voided.id.eqn(4));
      assert.ok(voided.outcome.voided.reason.governance);
      assert.ok(
        (await program.account.transferStatus.fetch(status(new BN(4)))).outcome
          .voided?.reason.governance,
      );
      await program.methods
        .setBridgeEnabled(true)
        .accountsStrict(management)
        .rpc();
      await rejectsWithProgramLogs(() => mintRecord(4), /already in use/);
    },
  );
  await t.test(
    "retries transfer limits and daily caps without voiding or resetting usage",
    async () => {
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .setMintPolicy({ ...policy, perTransferMax: new BN(2_000_000_000) })
            .accountsStrict(management)
            .rpc(),
        "InvalidPolicy",
      );
      await rejectsWithAnchorCode(() => mintRecord(2), "TransferLimit");
      await assertUnresolved(2);
      const raised = { ...policy, perTransferMax: new BN(800_000_000) };
      const changed = await emittedEvent(
        program,
        await program.methods
          .setMintPolicy(raised)
          .accountsStrict(management)
          .rpc(),
        "mintPolicySet",
      );
      assertSamePolicy(changed.previous, policy);
      assertSamePolicy(changed.policy, raised);
      await mintRecord(2);
      await rejectsWithAnchorCode(() => mintRecord(5), "DailyLimit");
      await assertUnresolved(5);
      await program.methods
        .setMintPolicy({ ...raised, dailyMintCap: new BN(2_000_000_000) })
        .accountsStrict(management)
        .rpc();
      assert.equal(
        (
          await program.account.bridgeConfig.fetch(bridge)
        ).mintedToday.toString(),
        "950000000",
      );
      await mintRecord(5);
      assert.equal((await getMint(connection, mint)).supply, 1_050_000_000n);
    },
  );
  await t.test(
    "allows registered vault PDAs and voids unregistered PDAs",
    async () => {
      await mintRecord(7);
      assert.equal(
        (
          await getAccount(
            connection,
            getAssociatedTokenAddressSync(mint, vault, true),
          )
        ).amount,
        10_000_000n,
      );
      await mintRecord(8);
      assert.ok(
        (await program.account.transferStatus.fetch(status(new BN(8)))).outcome
          .voided,
      );
    },
  );
  await t.test(
    "returns upload rent only to its owner and retains permanent transfer status",
    async () => {
      const uploaded = await buffer(transferKey(1));
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .closeProofBuffer()
            .accountsStrict({
              submitter: outsider.publicKey,
              proofBuffer: uploaded,
            })
            .signers([outsider])
            .rpc(),
        "ConstraintHasOne",
      );
      await program.methods
        .closeProofBuffer()
        .accountsStrict({
          submitter: provider.publicKey,
          proofBuffer: uploaded,
        })
        .rpc();
      assert.equal(await connection.getAccountInfo(uploaded), null);
      assert.ok(
        (await program.account.transferStatus.fetch(status(new BN(1)))).outcome
          .minted,
      );
    },
  );
  await t.test(
    "rejects an expired client even though its membership proof remains valid",
    async () => {
      const value = record(9);
      const accounts = mintAccounts(value, await buffer(transferKey(9)));
      const expiry = timestamp + trustingPeriod;
      const deadline = Date.now() + 90_000;
      // Poll the validator clock. This exercises our expiry policy with real accounts.
      while (true) {
        assert.ok(
          Date.now() < deadline && !t.signal.aborted,
          "validator clock did not reach client expiry",
        );
        const slot = await connection.getSlot();
        const now = await connection.getBlockTime(slot);
        if (now !== null && now >= expiry) break;
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(value, HEIGHT)
            .accountsStrict(accounts)
            .preInstructions(compute())
            .rpc(),
        "ClientExpired",
      );
      await assertUnresolved(9);
      assert.equal((await getMint(connection, mint)).supply, 1_060_000_000n);
    },
  );
  await t.test(
    "reporter reads finalized automatic and governance voids",
    async () => {
      const observer = new SolanaObserver(connection, {
        genesisHash: await connection.getGenesisHash(),
        programId: program.programId,
        mint,
        lightClientProgram: client.programId,
        akashChainId: CHAIN,
        maxAgeSeconds: 60,
      });
      for (const id of [3, 4]) {
        const observed = await observer.observe(
          decodeRecord(encodeRecord(record(id))),
        );
        assert.ok(observed.kind === "resolved");
        assert.equal(observed.observation.outcome, Outcome.OUTCOME_VOIDED);
        assert.equal(observed.observation.transactionSignature.length, 64);
      }
    },
  );
  await t.test(
    "re-pins away from an expired, closed deployment without replay",
    async () => {
      const before = await program.account.bridgeConfig.fetch(bridge);
      // The replacement must itself be active.
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .repinLightClient(CHAIN)
            .accountsStrict(repinAccounts(client.programId))
            .rpc(),
        "ClientExpired",
      );
      // Recovery never reads the pinned deployment, so closing it cannot
      // block a re-pin. Upgradeable loader Close is instruction 5.
      const pinnedData = programData(client.programId);
      await provider.sendAndConfirm(
        new web3.Transaction().add(
          new web3.TransactionInstruction({
            programId: LOADER,
            keys: [
              { pubkey: pinnedData, isSigner: false, isWritable: true },
              { pubkey: provider.publicKey, isSigner: false, isWritable: true },
              { pubkey: provider.publicKey, isSigner: true, isWritable: false },
              { pubkey: client.programId, isSigner: false, isWritable: true },
            ],
            data: u32(5),
          }),
        ),
      );
      assert.equal(await connection.getAccountInfo(pinnedData), null);

      const event = await repin(REPIN_CLIENT);
      assert.ok(event.previousProgram.equals(client.programId));
      assert.ok(event.program.equals(REPIN_CLIENT));
      const after = await program.account.bridgeConfig.fetch(bridge);
      assert.ok(after.lightClientProgram.equals(REPIN_CLIENT));
      assert.equal(after.chainId, CHAIN);
      assert.equal(JSON.stringify(after.policy), JSON.stringify(before.policy));
      assert.equal(after.mintDay.toString(), before.mintDay.toString());
      assert.equal(after.mintedToday.toString(), before.mintedToday.toString());

      const replacementClientAccounts = {
        lightClientProgram: REPIN_CLIENT,
        clientState: replacementClient,
        latestConsensus: replacementConsensus,
        proofConsensus: replacementConsensus,
      };
      const replacementAccounts = (
        value: ReturnType<typeof makeRecord>,
        proofBuffer: web3.PublicKey,
      ) => ({
        ...mintAccounts(value, proofBuffer),
        ...replacementClientAccounts,
      });
      // The transfer blocked by expiry now mints through the new client.
      await program.methods
        .mintTransfer(record(9), HEIGHT)
        .accountsStrict(
          replacementAccounts(record(9), await buffer(transferKey(9))),
        )
        .preInstructions(compute())
        .rpc();
      assert.equal((await getMint(connection, mint)).supply, 1_070_000_000n);
      // Statuses are keyed by record ID only, so earlier mints and voids stay
      // final.
      const first = await upload(proof(transferKey(1)));
      await rejectsWithProgramLogs(
        () =>
          program.methods
            .mintTransfer(record(1), HEIGHT)
            .accountsStrict(replacementAccounts(record(1), first))
            .preInstructions(compute())
            .rpc(),
        /already in use/,
      );
      await rejectsWithProgramLogs(
        async () =>
          program.methods
            .voidTransfer(new BN(4), HEIGHT)
            .accountsStrict({
              ...(await voidAccounts(4)),
              ...replacementClientAccounts,
            })
            .preInstructions(compute())
            .rpc(),
        /already in use/,
      );
      // The previous deployment is no longer accepted.
      const sixth = await buffer(transferKey(6));
      await rejectsWithAnchorCode(
        () =>
          program.methods
            .mintTransfer(record(6), HEIGHT)
            .accountsStrict(mintAccounts(record(6), sixth))
            .preInstructions(compute())
            .rpc(),
        "ConstraintAddress",
      );
      assert.equal((await getMint(connection, mint)).supply, 1_070_000_000n);
    },
  );
}
