import assert from "node:assert/strict";
import { test } from "node:test";
import {
  AnchorProvider,
  BN,
  BorshCoder,
  EventParser,
  Program,
  Wallet,
  web3,
  type Idl,
} from "@coral-xyz/anchor";
import aktIdl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { AktSolana } from "../target/types/akt_solana";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import {
  ADMIN_ROLE,
  BPF_LOADER_UPGRADEABLE,
  IdlProgram,
  RELAYER_ROLE,
  accessManager,
  accessManagerState,
  agreeCheckpoint,
  akt,
  aktAddresses,
  bundleFiles,
  checkpointFromCommit,
  clientAddresses,
  configureBridge,
  decodeEvents,
  decodeInstruction,
  emitFile,
  flattenAccounts,
  grantRole,
  guardianPause,
  initAccessManager,
  initClient,
  initialize,
  lightClient,
  parseCheckpoint,
  readEmitFile,
  renderSummary,
  repinLightClient,
  revisionNumber,
  revokeRole,
  rfc3339Nanos,
  setBridgeEnabled,
  setMintPolicy,
  setPauseGuardian,
  setTokenMetadata,
  upgradeProgram,
  viaCpiProxy,
  type Checkpoint,
  type ProgramName,
} from "./admin-instructions";
import { programData } from "./common";

// Nothing here contacts a cluster; the provider only satisfies Anchor.
const provider = new AnchorProvider(
  new web3.Connection("http://127.0.0.1:9"),
  new Wallet(web3.Keypair.generate()),
  {},
);
const oracle = {
  akt: new Program<AktSolana>(aktIdl, provider),
  client: new Program<Ics07Tendermint>(clientIdl, provider),
  access: new Program<AccessManager>(accessIdl, provider),
};
const key = () => web3.Keypair.generate().publicKey;
const vault = key();
const AKT = akt();
const CLIENT = lightClient();
const ACCESS = accessManager();
const genesisHash = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG";
const checkpoint: Checkpoint = {
  chainId: "akashnet-2",
  height: "23456789",
  time: "2026-10-19T12:00:01.123456789Z",
  timestamp: "1792411201123456789",
  root: "aa".repeat(32),
  nextValidatorsHash: "bb".repeat(32),
  validatorsHash: "cc".repeat(32),
  blockHash: "dd".repeat(32),
};
const parameters = {
  trustLevel: [1n, 3n] as [bigint, bigint],
  trustingPeriod: 1_209_600n,
  unbondingPeriod: 1_814_400n,
  maxClockDrift: 10n,
};

function same(
  actual: web3.TransactionInstruction,
  expected: web3.TransactionInstruction,
) {
  assert.equal(actual.programId.toBase58(), expected.programId.toBase58());
  assert.deepEqual(
    actual.keys.map((k) => [k.pubkey.toBase58(), k.isSigner, k.isWritable]),
    expected.keys.map((k) => [k.pubkey.toBase58(), k.isSigner, k.isWritable]),
  );
  assert.equal(actual.data.toString("hex"), expected.data.toString("hex"));
}

// A fresh coder from the raw IDL JSON, and the IDL's own account flags.
function roundTrip(
  ix: web3.TransactionInstruction,
  idl: unknown,
  name: string,
) {
  const raw = idl as Idl;
  const decoded = new BorshCoder(raw).instruction.decode(ix.data);
  assert.ok(decoded);
  assert.equal(decoded.name, name);
  const definition = raw.instructions.find((entry) => entry.name === name);
  assert.ok(definition);
  const flat = flattenAccounts(definition.accounts as never);
  assert.equal(ix.keys.length, flat.length);
  flat.forEach((account, index) => {
    assert.equal(ix.keys[index].isSigner, account.signer, account.name);
    assert.equal(ix.keys[index].isWritable, account.writable, account.name);
  });
  return decoded.data as Record<string, unknown>;
}

test("AKT admin instructions match Anchor's builder and decode back", async () => {
  const client = CLIENT.programId;
  const { clientState, consensusAt } = clientAddresses(client);
  const admin = {
    programData: programData(AKT.programId),
    authority: vault,
  };
  const policy = {
    perTransferMax: 1_000_000n,
    dailyMintCap: 5_000_000n,
    vaults: [key()],
  };
  const anchorPolicy = {
    perTransferMax: new BN(1_000_000),
    dailyMintCap: new BN(5_000_000),
    vaults: policy.vaults,
  };

  const genesis = initialize(AKT, vault);
  same(
    genesis,
    await oracle.akt.methods
      .initialize()
      .accountsPartial({ ...admin })
      .instruction(),
  );
  assert.deepEqual(roundTrip(genesis, aktIdl, "initialize"), {});
  assert.deepEqual(
    genesis.keys.slice(3, 6).map((meta) => meta.pubkey.toBase58()),
    [
      aktAddresses(AKT.programId).config,
      aktAddresses(AKT.programId).mint,
      aktAddresses(AKT.programId).mintAuthority,
    ].map((address) => address.toBase58()),
  );

  const configure = configureBridge(
    AKT,
    vault,
    client,
    77n,
    "akashnet-2",
    policy,
  );
  same(
    configure,
    await oracle.akt.methods
      .configureBridge("akashnet-2", anchorPolicy)
      .accountsPartial({
        admin,
        lightClientProgram: client,
        clientState,
        latestConsensus: consensusAt(77n),
      })
      .instruction(),
  );
  const args = roundTrip(configure, aktIdl, "configure_bridge");
  assert.equal(args.chain_id, "akashnet-2");
  const decodedPolicy = args.policy as Record<string, unknown>;
  assert.equal(String(decodedPolicy.per_transfer_max), "1000000");
  assert.equal(String(decodedPolicy.daily_mint_cap), "5000000");

  for (const enabled of [true, false]) {
    const ix = setBridgeEnabled(AKT, vault, enabled);
    same(
      ix,
      await oracle.akt.methods
        .setBridgeEnabled(enabled)
        .accountsPartial({ admin })
        .instruction(),
    );
    assert.deepEqual(roundTrip(ix, aktIdl, "set_bridge_enabled"), { enabled });
  }

  const policyIx = setMintPolicy(AKT, vault, policy);
  same(
    policyIx,
    await oracle.akt.methods
      .setMintPolicy(anchorPolicy)
      .accountsPartial({ admin })
      .instruction(),
  );

  for (const guardian of [key(), null]) {
    const ix = setPauseGuardian(AKT, vault, guardian);
    same(
      ix,
      await oracle.akt.methods
        .setPauseGuardian(guardian)
        .accountsPartial({ admin })
        .instruction(),
    );
    assert.deepEqual(decodeInstruction(ix, "akt_solana").args, {
      guardian: guardian?.toBase58() ?? null,
    });
  }

  const uri = "https://example.org/token/akt-v1.json";
  const metadata = setTokenMetadata(AKT, vault, uri);
  same(
    metadata,
    await oracle.akt.methods
      .setTokenMetadata(uri)
      .accountsPartial({ admin, mint: aktAddresses(AKT.programId).mint })
      .instruction(),
  );
  assert.deepEqual(roundTrip(metadata, aktIdl, "set_token_metadata"), { uri });

  const repin = repinLightClient(AKT, vault, client, 90n, "akashnet-2");
  same(
    repin,
    await oracle.akt.methods
      .repinLightClient("akashnet-2")
      .accountsPartial({
        admin,
        lightClientProgram: client,
        clientState,
        latestConsensus: consensusAt(90n),
      })
      .instruction(),
  );

  const guardian = key();
  same(
    guardianPause(AKT, guardian),
    await oracle.akt.methods
      .guardianPause()
      .accountsPartial({ guardian })
      .instruction(),
  );
});

test("access-manager and light-client instructions match Anchor's builder", async () => {
  const relayer = key();
  for (const [build, method] of [
    [grantRole, "grantRole"],
    [revokeRole, "revokeRole"],
  ] as const) {
    const ix = build(ACCESS, vault, RELAYER_ROLE, relayer);
    same(
      ix,
      await oracle.access.methods[method](new BN(1), relayer)
        .accountsPartial({ admin: vault })
        .instruction(),
    );
    const args = roundTrip(
      ix,
      accessIdl,
      method === "grantRole" ? "grant_role" : "revoke_role",
    );
    assert.equal(String(args.role_id), "1");
    assert.ok((args.account as web3.PublicKey).equals(relayer));
  }

  const ceremony = key();
  const squads = key();
  const bootstrap = initAccessManager(
    ACCESS,
    ceremony,
    vault,
    [squads],
    [relayer],
  );
  assert.deepEqual(
    bootstrap.map((ix) => decodeInstruction(ix, "access_manager").instruction),
    [
      "initialize",
      "set_whitelisted_programs",
      "grant_role",
      "grant_role",
      "renounce_role",
    ],
  );
  same(
    bootstrap[0],
    await oracle.access.methods
      .initialize(ceremony)
      .accountsPartial({ payer: ceremony, authority: ceremony })
      .instruction(),
  );
  same(
    bootstrap[1],
    await oracle.access.methods
      .setWhitelistedPrograms([squads])
      .accountsPartial({ admin: ceremony })
      .instruction(),
  );
  same(
    bootstrap[4],
    await oracle.access.methods
      .renounceRole(new BN(0))
      .accountsPartial({ caller: ceremony })
      .instruction(),
  );
  const handover = decodeInstruction(bootstrap[3], "access_manager");
  assert.deepEqual(handover.args, {
    role_id: ADMIN_ROLE.toString(),
    account: vault.toBase58(),
  });
  // The ceremony key signs everything; it never names the vault as signer.
  for (const ix of bootstrap)
    for (const meta of ix.keys)
      if (meta.isSigner) assert.ok(meta.pubkey.equals(ceremony));
  // A ceremony key that stays admin needs only the initialize.
  assert.equal(initAccessManager(ACCESS, ceremony, ceremony, []).length, 1);

  const manager = ACCESS.programId;
  const client = initClient(
    CLIENT,
    vault,
    vault,
    manager,
    checkpoint,
    parameters,
  );
  const clientState = {
    chainId: "akashnet-2",
    trustLevelNumerator: new BN(1),
    trustLevelDenominator: new BN(3),
    trustingPeriod: new BN(1_209_600),
    unbondingPeriod: new BN(1_814_400),
    maxClockDrift: new BN(10),
    frozenHeight: { revisionNumber: new BN(0), revisionHeight: new BN(0) },
    latestHeight: {
      revisionNumber: new BN(2),
      revisionHeight: new BN(23456789),
    },
  };
  same(
    client,
    await oracle.client.methods
      .initialize(
        clientState,
        {
          timestamp: new BN("1792411201123456789"),
          root: Array(32).fill(0xaa),
          nextValidatorsHash: Array(32).fill(0xbb),
        },
        manager,
      )
      .accountsPartial({ payer: vault, authority: vault })
      .instruction(),
  );
  const decoded = decodeInstruction(client, "ics07_tendermint");
  assert.deepEqual(decoded.args, {
    client_state: {
      chain_id: "akashnet-2",
      trust_level_numerator: "1",
      trust_level_denominator: "3",
      trusting_period: "1209600",
      unbonding_period: "1814400",
      max_clock_drift: "10",
      frozen_height: { revision_number: "0", revision_height: "0" },
      latest_height: { revision_number: "2", revision_height: "23456789" },
    },
    consensus_state: {
      timestamp: "1792411201123456789",
      root: "aa".repeat(32),
      next_validators_hash: "bb".repeat(32),
    },
    access_manager: manager.toBase58(),
  });
});

test("fresh program IDs move every address and fixed account", () => {
  const aktId = key();
  const clientId = key();
  const accessId = key();
  const program = akt(aktId);
  const ix = setBridgeEnabled(program, vault, true);
  assert.ok(ix.programId.equals(aktId));
  assert.ok(ix.keys[0].pubkey.equals(aktId));
  assert.ok(ix.keys[1].pubkey.equals(programData(aktId)));
  assert.ok(ix.keys[3].pubkey.equals(aktAddresses(aktId).config));
  const decoded = decodeInstruction(ix, "akt_solana");
  assert.equal(decoded.accounts[0].name, "admin.program");
  assert.equal(decoded.accounts[0].pubkey, aktId.toBase58());
  // The default deployment's address is no longer accepted for this program.
  assert.throws(
    () =>
      decodeInstruction(
        new web3.TransactionInstruction({ ...ix, programId: AKT.programId }),
        "akt_solana",
      ),
    /admin.program must be/,
  );

  const client = initClient(
    lightClient(clientId),
    vault,
    vault,
    accessId,
    checkpoint,
    parameters,
  );
  assert.ok(
    client.keys[0].pubkey.equals(clientAddresses(clientId).clientState),
  );
  assert.ok(
    client.keys[1].pubkey.equals(
      clientAddresses(clientId).consensusAt(23456789n),
    ),
  );
  assert.ok(client.keys[5].pubkey.equals(programData(clientId)));
  const role = grantRole(
    accessManager(accessId),
    vault,
    RELAYER_ROLE,
    clientId,
  );
  assert.ok(role.keys[0].pubkey.equals(accessManagerState(accessId)));

  const file = emitFile({
    command: "test",
    genesisHash,
    authority: vault,
    instructions: [client, role, ix],
    programs: {
      akt_solana: aktId,
      ics07_tendermint: clientId,
      access_manager: accessId,
    },
  });
  assert.deepEqual(
    file.summary.map((entry) => entry.program),
    ["ics07_tendermint", "access_manager", "akt_solana"],
  );
  // Without the deployment map the file cannot be attributed.
  assert.throws(
    () =>
      emitFile({
        command: "test",
        genesisHash,
        authority: vault,
        instructions: [ix],
        programs: {},
      }),
    /unknown program/,
  );
});

test("builders reject what the programs would reject", () => {
  assert.throws(
    () =>
      setMintPolicy(AKT, vault, {
        perTransferMax: 6n,
        dailyMintCap: 5n,
        vaults: [],
      }),
    /at most the daily cap/,
  );
  assert.throws(
    () =>
      setMintPolicy(AKT, vault, {
        perTransferMax: 0n,
        dailyMintCap: 5n,
        vaults: [],
      }),
    /positive/,
  );
  assert.throws(
    () =>
      setMintPolicy(AKT, vault, {
        perTransferMax: 1n,
        dailyMintCap: 5n,
        vaults: [web3.PublicKey.default],
      }),
    /zero address/,
  );
  assert.throws(
    () =>
      setMintPolicy(AKT, vault, {
        perTransferMax: 1n,
        dailyMintCap: 5n,
        vaults: Array.from({ length: 17 }, key),
      }),
    /at most 16 vaults/,
  );
  assert.throws(
    () => setTokenMetadata(AKT, vault, "http://example.org/a.json"),
    /HTTPS/,
  );
  assert.throws(
    () =>
      setTokenMetadata(AKT, vault, `https://example.org/${"a".repeat(190)}`),
    /200 bytes/,
  );
  assert.throws(
    () =>
      configureBridge(AKT, vault, key(), 1n, "x".repeat(65), {
        perTransferMax: 1n,
        dailyMintCap: 1n,
        vaults: [],
      }),
    /chain ID/,
  );
  assert.throws(
    () =>
      initClient(CLIENT, vault, vault, key(), checkpoint, {
        ...parameters,
        trustingPeriod: 1_814_400n,
      }),
    /below the unbonding period/,
  );
  assert.throws(
    () =>
      initClient(CLIENT, vault, vault, key(), checkpoint, {
        ...parameters,
        trustLevel: [2n, 1n],
      }),
    /trust level/,
  );
  // Strict account lists: missing, unknown and misplaced fixed addresses.
  assert.throws(
    () => AKT.instruction("set_bridge_enabled", { enabled: true }, {}),
    /missing admin.program/,
  );
  assert.throws(
    () =>
      AKT.instruction(
        "guardian_pause",
        {},
        { guardian: vault, pause_guardian: key(), config: key(), extra: key() },
      ),
    /unknown accounts extra/,
  );
  assert.throws(
    () =>
      AKT.instruction(
        "set_pause_guardian",
        { guardian: null },
        {
          admin: {
            program: AKT.programId,
            program_data: key(),
            authority: vault,
          },
          pause_guardian: key(),
          system_program: key(),
        },
      ),
    /system_program must be 11111111111111111111111111111111/,
  );
  assert.throws(
    () => AKT.instruction("set_bridge_enabled", {}, {}),
    /arguments differ/,
  );
});

test("emitted files round-trip and reject tampering", () => {
  const instructions = [
    setPauseGuardian(AKT, vault, key()),
    setMintPolicy(AKT, vault, {
      perTransferMax: 1n,
      dailyMintCap: 2n,
      vaults: [],
    }),
  ];
  const file = emitFile({
    command: "test",
    genesisHash,
    authority: vault,
    instructions,
    programs: {},
    checks: { before: new BN(5), key: vault },
    notes: ["execute soon"],
  });
  assert.deepEqual(file.checks, { before: "5", key: vault.toBase58() });
  const json = JSON.parse(JSON.stringify(file));
  const read = readEmitFile(json);
  read.instructions.forEach((ix, index) => same(ix, instructions[index]));
  assert.ok(read.authority.equals(vault));
  const text = renderSummary(file);
  assert.match(text, /1\. akt_solana\.set_pause_guardian/);
  assert.match(
    text,
    new RegExp(`admin\\.authority\\s+${vault.toBase58()}  signer writable`),
  );
  assert.match(text, /Note: execute soon/);

  // Change the per-transfer maximum, then append a byte the IDL lacks.
  const tampered = structuredClone(json);
  const data = Buffer.from(tampered.instructions[1].data, "base64");
  data[8] ^= 1;
  tampered.instructions[1].data = data.toString("base64");
  assert.throws(() => readEmitFile(tampered), /summary does not match/);
  const trailing = structuredClone(json);
  trailing.instructions[1].data = Buffer.concat([
    Buffer.from(trailing.instructions[1].data, "base64"),
    Buffer.from([0]),
  ]).toString("base64");
  assert.throws(
    () => readEmitFile(trailing),
    /bytes the IDL does not describe/,
  );

  const flags = structuredClone(json);
  flags.instructions[0].accounts[3].isWritable = false;
  assert.throws(() => readEmitFile(flags), /writable flag/);

  const signer = structuredClone(json);
  signer.instructions[0].accounts[3].isSigner = true;
  assert.throws(() => readEmitFile(signer), /only the authority may sign/);

  assert.throws(
    () => readEmitFile({ ...json, format: "other" }),
    /not an akt-solana-admin-instructions\/v1 file/,
  );
});

test("only the authority signs emitted instructions; bundles keep order", () => {
  assert.throws(
    () =>
      emitFile({
        command: "pause",
        genesisHash,
        authority: vault,
        instructions: [guardianPause(AKT, key())],
        programs: {},
      }),
    /signer other than the authority/,
  );
  const client = emitFile({
    command: "init-client",
    genesisHash,
    authority: vault,
    instructions: [
      initClient(
        CLIENT,
        vault,
        vault,
        ACCESS.programId,
        checkpoint,
        parameters,
      ),
    ],
    programs: {},
  });
  const configure = emitFile({
    command: "configure-bridge",
    genesisHash,
    authority: vault,
    instructions: [
      configureBridge(AKT, vault, CLIENT.programId, 23456789n, "akashnet-2", {
        perTransferMax: 1n,
        dailyMintCap: 1n,
        vaults: [],
      }),
    ],
    programs: {},
  });
  const bundle = bundleFiles([client, configure]);
  assert.equal(bundle.command, "init-client + configure-bridge");
  assert.deepEqual(
    readEmitFile(bundle).instructions.map((ix) => ix.programId.toBase58()),
    [CLIENT.programId.toBase58(), AKT.programId.toBase58()],
  );
  // configure_bridge names the consensus state the initialize creates.
  assert.equal(
    bundle.summary[1].accounts[7].pubkey,
    bundle.summary[0].accounts[1].pubkey,
  );
  assert.throws(
    () => bundleFiles([client, { ...configure, genesisHash: "other" }]),
    /different clusters/,
  );
});

test("the CPI proxy wrapper matches upstream's proxy_cpi layout", () => {
  const proxy = key();
  const inner = setBridgeEnabled(AKT, vault, true);
  const wrapped = viaCpiProxy(proxy, inner, vault);
  assert.ok(wrapped.programId.equals(proxy));
  assert.deepEqual(
    wrapped.keys
      .slice(0, 2)
      .map((k) => [k.pubkey.toBase58(), k.isSigner, k.isWritable]),
    [
      [AKT.programId.toBase58(), false, false],
      [vault.toBase58(), true, true],
    ],
  );
  assert.deepEqual(wrapped.keys.slice(2), inner.keys);
  const data = wrapped.data;
  // proxy_cpi's discriminator in upstream's test_cpi_proxy IDL.
  assert.equal(data.subarray(0, 8).toString("hex"), "59d65371457101ed");
  assert.equal(data.readUInt32LE(8), inner.data.length);
  assert.ok(data.subarray(12, 12 + inner.data.length).equals(inner.data));
  const metas = 12 + inner.data.length;
  assert.equal(data.readUInt32LE(metas), inner.keys.length);
  assert.deepEqual(
    [...data.subarray(metas + 4)],
    inner.keys.flatMap((k) => [Number(k.isSigner), Number(k.isWritable)]),
  );
});

test("program upgrade instruction uses the loader layout", () => {
  const program = key();
  const buffer = key();
  const spill = key();
  const ix = upgradeProgram(program, buffer, spill, vault);
  assert.ok(ix.programId.equals(BPF_LOADER_UPGRADEABLE));
  assert.deepEqual([...ix.data], [3, 0, 0, 0]);
  const decoded = decodeInstruction(ix, "bpf_loader_upgradeable");
  assert.deepEqual(
    decoded.accounts.map((a) => [a.name, a.signer, a.writable]),
    [
      ["program_data", false, true],
      ["program", false, true],
      ["buffer", false, true],
      ["spill", false, true],
      ["rent", false, false],
      ["clock", false, false],
      ["authority", true, false],
    ],
  );
  assert.equal(decoded.accounts[0].pubkey, programData(program).toBase58());
  const file = emitFile({
    command: "upgrade",
    genesisHash,
    authority: vault,
    instructions: [ix],
    programs: {},
  });
  same(readEmitFile(JSON.parse(JSON.stringify(file))).instructions[0], ix);
  const wrong = new web3.TransactionInstruction({
    ...ix,
    keys: ix.keys.map((meta, index) =>
      index === 0 ? { ...meta, pubkey: key() } : meta,
    ),
  });
  assert.throws(
    () => decodeInstruction(wrong, "bpf_loader_upgradeable"),
    /does not belong/,
  );
});

test("events are read by invocation frame, also inside a CPI", () => {
  const multisig = key();
  const other = key();
  const definition = (aktIdl as unknown as Idl).events?.find(
    (event) => event.name === "MintPolicySet",
  );
  assert.ok(definition);
  const policy = {
    per_transfer_max: new BN(1),
    daily_mint_cap: new BN(2),
    vaults: [vault],
  };
  const data = Buffer.concat([
    Buffer.from(definition.discriminator),
    AKT.coder.types.encode("MintPolicySet", {
      previous: policy,
      policy: { ...policy, per_transfer_max: new BN(2) },
    }),
  ]).toString("base64");
  const aktId = AKT.programId.toBase58();
  const inner = [
    `Program ${aktId} invoke [2]`,
    "Program log: Instruction: SetMintPolicy",
    `Program data: ${data}`,
    `Program ${aktId} consumed 6000 of 194000 compute units`,
    `Program ${aktId} success`,
  ];
  // As a multisig executes it: the AKT frame runs inside the multisig's.
  // Another program's data line at the same depth is not an AKT event.
  const cpi = [
    `Program ${multisig} invoke [1]`,
    "Program log: Instruction: VaultTransactionExecute",
    ...inner,
    `Program ${other} invoke [2]`,
    `Program data: ${data}`,
    `Program ${other} success`,
    `Program ${multisig} consumed 30000 of 200000 compute units`,
    `Program ${multisig} success`,
  ];
  const expected = {
    name: "MintPolicySet",
    data: {
      previous: {
        per_transfer_max: "1",
        daily_mint_cap: "2",
        vaults: [vault.toBase58()],
      },
      policy: {
        per_transfer_max: "2",
        daily_mint_cap: "2",
        vaults: [vault.toBase58()],
      },
    },
  };
  assert.deepEqual(decodeEvents(cpi, AKT), [expected]);
  // Anchor's parser misses the event inside the CPI, not at top level.
  const parser = new EventParser(AKT.programId, oracle.akt.coder);
  assert.equal([...parser.parseLogs(cpi)].length, 0);
  const direct = inner.map((line) => line.replace("invoke [2]", "invoke [1]"));
  assert.equal([...parser.parseLogs(direct)].length, 1);
  assert.deepEqual(decodeEvents(direct, AKT), [expected]);
  // Events of a deployment at another address are not this one's.
  assert.deepEqual(decodeEvents(cpi, akt(key())), []);
  assert.throws(
    () => decodeEvents(cpi.slice(0, -1), AKT),
    /inconsistent program log stack/,
  );
});

test("checkpoints come from agreeing CometBFT commits", () => {
  assert.equal(
    rfc3339Nanos("2026-10-19T12:00:01.123456789Z"),
    1792411201123456789n,
  );
  assert.equal(rfc3339Nanos("2026-10-19T12:00:01.5Z"), 1792411201500000000n);
  assert.equal(rfc3339Nanos("2026-10-19T12:00:01Z"), 1792411201000000000n);
  assert.throws(() => rfc3339Nanos("2026-10-19T12:00:01+02:00"), /UTC/);
  assert.equal(revisionNumber("akashnet-2"), 2n);
  assert.equal(revisionNumber("akash-bridge-testnetify-team-1"), 1n);
  assert.equal(revisionNumber("local"), 0n);
  assert.equal(revisionNumber("chain-01"), 0n);

  const commit = (overrides: Record<string, string> = {}) => ({
    jsonrpc: "2.0",
    id: -1,
    result: {
      canonical: true,
      signed_header: {
        header: {
          chain_id: "akashnet-2",
          height: "23456789",
          time: "2026-10-19T12:00:01.123456789Z",
          app_hash: "AA".repeat(32),
          next_validators_hash: "BB".repeat(32),
          validators_hash: "CC".repeat(32),
          ...overrides,
        },
        commit: { height: "23456789", block_id: { hash: "DD".repeat(32) } },
      },
    },
  });
  const parsed = checkpointFromCommit(commit());
  assert.deepEqual(parsed, checkpoint);
  const agreed = agreeCheckpoint(
    ["https://a", "https://b", "https://c"].map((rpc) => ({
      rpc,
      fetchedAt: "2026-10-19T12:01:00.000Z",
      checkpoint: checkpointFromCommit(commit()),
    })),
  );
  assert.equal(agreed.sources?.length, 3);
  assert.deepEqual(parseCheckpoint(JSON.parse(JSON.stringify(agreed))), agreed);
  assert.throws(
    () =>
      agreeCheckpoint([
        { rpc: "https://a", fetchedAt: "t", checkpoint: parsed },
        {
          rpc: "https://b",
          fetchedAt: "t",
          checkpoint: checkpointFromCommit(
            commit({ app_hash: "AB".repeat(32) }),
          ),
        },
      ]),
    /https:\/\/b reports a different root/,
  );
  const pending = commit();
  pending.result.canonical = false;
  assert.throws(() => checkpointFromCommit(pending), /not canonical/);
  assert.throws(
    () => parseCheckpoint({ ...checkpoint, timestamp: "1792411201123456788" }),
    /does not match time/,
  );
});

test("every listed admin instruction exists in the IDLs", () => {
  const expected: [ProgramName, string][] = [
    ["akt_solana", "initialize"],
    ["akt_solana", "set_token_metadata"],
    ["akt_solana", "configure_bridge"],
    ["akt_solana", "set_bridge_enabled"],
    ["akt_solana", "set_mint_policy"],
    ["akt_solana", "set_pause_guardian"],
    ["akt_solana", "repin_light_client"],
    ["akt_solana", "guardian_pause"],
    ["access_manager", "initialize"],
    ["access_manager", "set_whitelisted_programs"],
    ["access_manager", "grant_role"],
    ["access_manager", "revoke_role"],
    ["access_manager", "renounce_role"],
    ["ics07_tendermint", "initialize"],
  ];
  for (const [program, name] of expected)
    assert.ok(new IdlProgram(program).accounts(name).length > 0);
});
