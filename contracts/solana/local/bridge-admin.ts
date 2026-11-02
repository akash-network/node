import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { AnchorProvider, Program, web3 } from "@coral-xyz/anchor";
import { getMint } from "@solana/spl-token";
import idl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { AktSolana } from "../target/types/akt_solana";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import { readTokenMetadata } from "../shared/token-metadata";
import * as ix from "./admin-instructions";
import { LE64, PROGRAM_ID, ROOT, SOLANA_RPC, json, pda } from "./common";

// Bridge administration. Every chain command needs the verified Solana genesis
// hash, so it cannot act on another cluster. Program addresses come from
// chain where possible: the bridge PDA pins the light client, and the
// client's app_state names its access manager.
//
// Signing. By default the team testnet's runtime key signs and sends. Use
// --keypair for another key (the launch ceremony key, a rehearsal vault).
// With --emit FILE --authority KEY nothing is signed or sent: the instructions
// are written for KEY, a multisig vault, to execute after review. Mainnet
// accepts local signatures only for the ceremony steps and the guardian pause.
//
// Instruction commands (each also takes --emit):
//   initialize
//   init-access-manager --admin KEY [--cpi-caller ID]... [--relayer KEY]...
//       [--access-manager ID]                       (local key only)
//   set-token-metadata --uri URL
//   init-client --checkpoint FILE --access-manager ID --trusting-period S
//       --unbonding-period S --max-clock-drift S [--trust-level 1/3] [--program ID]
//   configure-bridge --program ID --chain-id ID --per-transfer-max N
//       --daily-mint-cap N [--vault KEY]... [--checkpoint FILE]
//   set-bridge-enabled --enabled true|false
//   set-mint-policy --per-transfer-max N --daily-mint-cap N [--vault KEY]...
//   set-guardian --guardian KEY | clear-guardian
//   pause [--guardian-keypair PATH]
//   repin --program ID [--chain-id ID] [--checkpoint FILE]
//   grant-relayer --key KEY | revoke-relayer --key KEY [--program ID]
//   open-relaying | close-relaying [--program ID]
//   upgrade-program --program ID --buffer KEY [--spill KEY] [--expect-sha256 HEX]
// Read-only:
//   show [--program ID] [--access-manager ID] | relayers [--program ID]
//   events --signature SIG    AKT events of a transaction, also inside a CPI
// Offline:
//   checkpoint --height H --akash-rpc URL... --out FILE
//   bundle --emit FILE IN.json IN.json...
//   decode FILE
// Rehearsal only, never mainnet:
//   execute FILE --keypair PATH [--cpi-proxy ID]
//
// Common: --genesis HASH [--url RPC] [--akt-program ID]. Amounts are base
// units (1 AKT = 1000000). See docs/deploy-program.md.
const commitment = "finalized";
const confirm = { commitment, preflightCommitment: commitment } as const;
type Wallet = ConstructorParameters<typeof AnchorProvider>[1];
type Values = ReturnType<typeof options>["values"];

function options() {
  return parseArgs({
    allowPositionals: true,
    options: {
      genesis: { type: "string" },
      url: { type: "string" },
      keypair: { type: "string" },
      emit: { type: "string" },
      authority: { type: "string" },
      "akt-program": { type: "string" },
      program: { type: "string" },
      // Earlier name for --program.
      "client-program": { type: "string" },
      "access-manager": { type: "string" },
      guardian: { type: "string" },
      "guardian-keypair": { type: "string" },
      "chain-id": { type: "string" },
      key: { type: "string" },
      admin: { type: "string" },
      "cpi-caller": { type: "string", multiple: true },
      relayer: { type: "string", multiple: true },
      uri: { type: "string" },
      enabled: { type: "string" },
      "per-transfer-max": { type: "string" },
      "daily-mint-cap": { type: "string" },
      vault: { type: "string", multiple: true },
      checkpoint: { type: "string" },
      "trust-level": { type: "string" },
      "trusting-period": { type: "string" },
      "unbonding-period": { type: "string" },
      "max-clock-drift": { type: "string" },
      buffer: { type: "string" },
      spill: { type: "string" },
      "expect-sha256": { type: "string" },
      height: { type: "string" },
      "akash-rpc": { type: "string", multiple: true },
      "cpi-proxy": { type: "string" },
      signature: { type: "string" },
      out: { type: "string" },
    },
  });
}

const failure = (error: unknown) => ({
  error: error instanceof Error ? error.message : String(error),
});

function pubkey(value: string | undefined, flag: string) {
  assert.ok(value, `Pass ${flag}`);
  return new web3.PublicKey(value);
}

async function keypair(path: string) {
  const raw = await json(path);
  assert.ok(
    Array.isArray(raw) &&
      raw.length === 64 &&
      raw.every(
        (v: unknown) =>
          typeof v === "number" && Number.isInteger(v) && v >= 0 && v < 256,
      ),
    "keypair file must hold 64 bytes",
  );
  return web3.Keypair.fromSecretKey(Uint8Array.from(raw));
}

// Reads chain state for a key this process does not hold.
function readOnly(publicKey: web3.PublicKey): Wallet {
  const refuse = async (): Promise<never> => {
    throw new Error("this command signs nothing");
  };
  return { publicKey, signTransaction: refuse, signAllTransactions: refuse };
}

// Never overwrite review artifacts.
async function create(path: string, value: unknown) {
  await writeFile(path, JSON.stringify(value, null, 2) + "\n", {
    flag: "wx",
    mode: 0o644,
  });
}

function clientProgram(wallet: AnchorProvider, address: web3.PublicKey) {
  return new Program<Ics07Tendermint>(
    { ...clientIdl, address: address.toBase58() },
    wallet,
  );
}

async function clientStatus(wallet: AnchorProvider, address: web3.PublicKey) {
  const client = clientProgram(wallet, address);
  const clientState = pda("client", address);
  const state = await client.account.clientState.fetch(clientState, commitment);
  const height = BigInt(state.latestHeight.revisionHeight.toString());
  const latestConsensus = pda("consensus_state", address, LE64(height));
  const consensus = await client.account.consensusStateStore.fetch(
    latestConsensus,
    commitment,
  );
  const now = await wallet.connection.getBlockTime(
    await wallet.connection.getSlot(commitment),
  );
  assert.ok(now !== null, "Solana clock unavailable");
  // Same rule as the AKT program: expired once now reaches time + trusting period.
  const time = BigInt(consensus.consensusState.timestamp.toString());
  const expires =
    time + BigInt(state.trustingPeriod.toString()) * 1_000_000_000n;
  const frozen =
    !state.frozenHeight.revisionHeight.isZero() ||
    !state.frozenHeight.revisionNumber.isZero();
  return {
    program: address.toBase58(),
    clientState,
    latestConsensus,
    chainId: state.chainId,
    latestHeight: height.toString(),
    consensusTime: new Date(Number(time / 1_000_000n)).toISOString(),
    expiresAt: new Date(Number(expires / 1_000_000n)).toISOString(),
    status: frozen
      ? "frozen"
      : BigInt(now) * 1_000_000_000n >= expires
        ? "expired"
        : "active",
  };
}

function printable(value: Awaited<ReturnType<typeof clientStatus>>) {
  const { clientState, latestConsensus, ...rest } = value;
  return {
    ...rest,
    clientState: clientState.toBase58(),
    latestConsensus: latestConsensus.toBase58(),
  };
}

async function accessState(
  wallet: AnchorProvider,
  managerProgram: web3.PublicKey,
) {
  const access = new Program<AccessManager>(
    { ...accessIdl, address: managerProgram.toBase58() },
    wallet,
  );
  const state = await access.account.accessManager.fetch(
    ix.accessManagerState(managerProgram),
    commitment,
  );
  const members = (role: bigint): web3.PublicKey[] =>
    state.roles.find((entry) => entry.roleId.toString() === role.toString())
      ?.members ?? [];
  return {
    program: managerProgram,
    admins: members(ix.ADMIN_ROLE),
    relayers: members(ix.RELAYER_ROLE),
    cpiCallers: state.whitelistedPrograms,
  };
}

const base58 = (keys: web3.PublicKey[]) => keys.map((key) => key.toBase58());

// The access manager is whatever the client's app_state names, not a constant.
async function relaying(wallet: AnchorProvider, address: web3.PublicKey) {
  const appState = await clientProgram(wallet, address).account.appState.fetch(
    pda("app_state", address),
    commitment,
  );
  const manager = await accessState(wallet, appState.amState.accessManager);
  return {
    ...manager,
    summary: {
      lightClientProgram: address.toBase58(),
      accessManagerProgram: manager.program.toBase58(),
      pendingAccessManager:
        appState.amState.pendingAccessManager?.toBase58() ?? null,
      openRelaying: manager.relayers.some((key) => key.equals(address)),
      relayers: base58(manager.relayers),
      admins: base58(manager.admins),
      cpiCallers: base58(manager.cpiCallers),
    },
  };
}

// ProgramData: u32 tag 3, u64 deploy slot, Option<Pubkey> authority, code.
async function upgradeInfo(
  connection: web3.Connection,
  program: web3.PublicKey,
) {
  const account = await connection.getAccountInfo(program, commitment);
  if (!account) return { program: program.toBase58(), deployed: false };
  assert.ok(
    account.executable && account.owner.equals(ix.BPF_LOADER_UPGRADEABLE),
    `${program.toBase58()} is not an upgradeable program`,
  );
  const dataAddress = new web3.PublicKey(account.data.subarray(4, 36));
  const header = await connection.getAccountInfo(dataAddress, {
    commitment,
    dataSlice: { offset: 0, length: 45 },
  });
  assert.ok(header && header.data.readUInt32LE(0) === 3, "ProgramData missing");
  return {
    program: program.toBase58(),
    deployed: true,
    programData: dataAddress.toBase58(),
    upgradeAuthority:
      header.data[12] === 1
        ? new web3.PublicKey(header.data.subarray(13, 45)).toBase58()
        : null,
    lastDeploySlot: header.data.readBigUInt64LE(4).toString(),
  };
}

interface Plan {
  instructions: web3.TransactionInstruction[];
  checks: Record<string, unknown>;
  notes?: string[];
  // Local mode only: keys that sign besides the fee payer.
  signers?: web3.Keypair[];
  // Local mode only: reads the result back after finalization.
  verify?: (signature: string) => Promise<Record<string, unknown>>;
  // Deployments other than the AKT program that the instructions call.
  deployments?: Partial<Record<ix.ProgramName, web3.PublicKey>>;
}

interface Context {
  values: Values;
  genesis: string;
  emit: boolean;
  authority: web3.PublicKey;
  signer?: web3.Keypair;
  wallet: AnchorProvider;
  connection: web3.Connection;
  aktId: web3.PublicKey;
  akt: ix.IdlProgram;
  program: Program<AktSolana>;
  addresses: ReturnType<typeof ix.aktAddresses>;
}

async function requireAuthority(
  ctx: Context,
  program: web3.PublicKey,
  label: string,
) {
  const info = await upgradeInfo(ctx.connection, program);
  assert.ok(info.deployed, `${label} ${program.toBase58()} is not deployed`);
  assert.equal(
    info.upgradeAuthority,
    ctx.authority.toBase58(),
    `${label} upgrade authority is ${info.upgradeAuthority}, not ${ctx.authority.toBase58()}`,
  );
  return info;
}

function policyFrom(values: Values): ix.MintPolicy {
  return {
    perTransferMax: ix.u64(values["per-transfer-max"], "--per-transfer-max"),
    dailyMintCap: ix.u64(values["daily-mint-cap"], "--daily-mint-cap"),
    vaults: (values.vault ?? []).map((v) => new web3.PublicKey(v)),
  };
}

const showPolicy = (policy: {
  perTransferMax: { toString(): string };
  dailyMintCap: { toString(): string };
  vaults: web3.PublicKey[];
}) => ({
  perTransferMax: policy.perTransferMax.toString(),
  dailyMintCap: policy.dailyMintCap.toString(),
  vaults: base58(policy.vaults),
});

async function checkpointFile(path: string | undefined) {
  assert.ok(path, "Pass --checkpoint with the ceremony's checkpoint file");
  return ix.parseCheckpoint(await json(path));
}

async function launch(ctx: Context) {
  const config = await ctx.program.account.launchConfig.fetchNullable(
    ctx.addresses.config,
    commitment,
  );
  assert.ok(config, "AKT is not initialized");
  return config;
}

async function bridgeConfig(ctx: Context) {
  const bridge = await ctx.program.account.bridgeConfig.fetchNullable(
    ctx.addresses.bridge,
    commitment,
  );
  assert.ok(bridge, "the bridge is not configured");
  return bridge;
}

async function guardianKey(ctx: Context) {
  return (
    (
      await ctx.program.account.pauseGuardian.fetchNullable(
        ctx.addresses.guardian,
        commitment,
      )
    )?.guardian?.toBase58() ?? null
  );
}

async function token(ctx: Context) {
  const config = await launch(ctx);
  const mint = await getMint(ctx.connection, config.mint, commitment);
  return {
    mint: config.mint.toBase58(),
    initializer: config.initializer.toBase58(),
    decimals: mint.decimals,
    supply: mint.supply.toString(),
    mintAuthority: mint.mintAuthority?.toBase58() ?? null,
    freezeAuthority: mint.freezeAuthority?.toBase58() ?? null,
    mintAuthorityIsPda:
      mint.mintAuthority?.equals(ctx.addresses.mintAuthority) ?? false,
  };
}

// configure_bridge and repin_light_client name the consensus state at the
// client's latest height. With a checkpoint, that is the checkpoint height,
// for a client the same multisig transaction initializes.
async function clientTarget(
  ctx: Context,
  client: web3.PublicKey,
  chain: string,
) {
  const notes: string[] = [];
  if (ctx.values.checkpoint) {
    const checkpoint = await checkpointFile(ctx.values.checkpoint);
    assert.equal(checkpoint.chainId, chain, "checkpoint names another chain");
    const existing = await ctx.connection.getAccountInfo(
      pda("client", client),
      commitment,
    );
    assert.ok(
      existing || ctx.emit,
      "the client is not initialized; run init-client first",
    );
    if (existing) {
      const status = await clientStatus(ctx.wallet, client);
      assert.equal(
        status.latestHeight,
        checkpoint.height,
        "the client has moved past the checkpoint; use its latest height",
      );
    } else
      notes.push(
        `The client is not initialized. Bundle this after init-client from the same checkpoint (height ${checkpoint.height}) in one multisig transaction.`,
      );
    return {
      latestHeight: BigInt(checkpoint.height),
      checks: { checkpointHeight: checkpoint.height, clientExists: !!existing },
      notes,
    };
  }
  const status = await clientStatus(ctx.wallet, client);
  assert.equal(status.status, "active", "the client must be active");
  assert.equal(status.chainId, chain, "the client names another chain");
  if (ctx.emit)
    notes.push(
      `Execution fails once the client moves past height ${status.latestHeight} or expires (${status.expiresAt}). Hold header updates for this client until the multisig executes, or bundle with a fresh init-client.`,
    );
  return {
    latestHeight: BigInt(status.latestHeight),
    checks: { client: printable(status) },
    notes,
  };
}

async function setRelayerPlan(
  ctx: Context,
  client: web3.PublicKey,
  key: web3.PublicKey,
  member: boolean,
): Promise<Plan> {
  const before = await relaying(ctx.wallet, client);
  const current = before.relayers.some((entry) => entry.equals(key));
  const checks = { key: key.toBase58(), member, ...before.summary };
  if (current === member) return { instructions: [], checks };
  assert.ok(
    before.admins.some((entry) => entry.equals(ctx.authority)),
    `${ctx.authority.toBase58()} lacks ADMIN_ROLE in this access manager`,
  );
  const notes =
    ctx.emit && before.cpiCallers.length === 0
      ? [
          "The access manager allows no CPI caller, so a multisig cannot execute this. set_whitelisted_programs must list the multisig program first.",
        ]
      : [];
  const build = member ? ix.grantRole : ix.revokeRole;
  return {
    instructions: [
      build(
        ix.accessManager(before.program),
        ctx.authority,
        ix.RELAYER_ROLE,
        key,
      ),
    ],
    checks,
    notes,
    deployments: { access_manager: before.program },
    verify: async (signature) => {
      const after = await relaying(ctx.wallet, client);
      assert.equal(
        after.relayers.some((entry) => entry.equals(key)),
        member,
      );
      return {
        key: key.toBase58(),
        member,
        changed: true,
        ...after.summary,
        signature,
      };
    },
  };
}

async function plan(command: string, ctx: Context): Promise<Plan> {
  const { values, authority, akt, program, addresses } = ctx;
  const named = values.program ?? values["client-program"];
  const pinnedClient = async () => (await bridgeConfig(ctx)).lightClientProgram;
  const targetClient = async () =>
    named ? new web3.PublicKey(named) : await pinnedClient();
  const setGuardian = async (guardian: web3.PublicKey | null) => {
    await requireAuthority(ctx, ctx.aktId, "AKT");
    const previous = await guardianKey(ctx);
    return {
      instructions: [ix.setPauseGuardian(akt, authority, guardian)],
      checks: { previous, guardian: guardian?.toBase58() ?? null },
      verify: async (signature: string) => {
        const current = await guardianKey(ctx);
        assert.equal(current, guardian?.toBase58() ?? null);
        return { previous, guardian: current, signature };
      },
    };
  };

  switch (command) {
    case "initialize": {
      assert.equal(
        await program.account.launchConfig.fetchNullable(
          addresses.config,
          commitment,
        ),
        null,
        "AKT is already initialized",
      );
      await requireAuthority(ctx, ctx.aktId, "AKT");
      return {
        instructions: [ix.initialize(akt, authority)],
        checks: {
          program: ctx.aktId,
          mint: addresses.mint,
          mintAuthority: addresses.mintAuthority,
        },
        verify: async (signature) => {
          const state = await token(ctx);
          assert.equal(state.decimals, 6);
          assert.equal(state.supply, "0");
          assert.equal(state.freezeAuthority, null);
          assert.ok(state.mintAuthorityIsPda);
          assert.equal((await launch(ctx)).bridgeEnabled, false);
          return { ...state, signature };
        },
      };
    }
    case "init-access-manager": {
      assert.ok(
        !ctx.emit,
        "The access manager's initialize rejects CPI: sign it with its upgrade authority (--keypair)",
      );
      const managerId = new web3.PublicKey(
        values["access-manager"] ?? accessIdl.address,
      );
      const finalAdmin = pubkey(
        values.admin,
        "--admin with the multisig vault",
      );
      const cpiCallers = (values["cpi-caller"] ?? []).map(
        (v) => new web3.PublicKey(v),
      );
      const relayers = (values.relayer ?? []).map((v) => new web3.PublicKey(v));
      assert.equal(
        await ctx.connection.getAccountInfo(
          ix.accessManagerState(managerId),
          commitment,
        ),
        null,
        "the access manager is already initialized",
      );
      await requireAuthority(ctx, managerId, "access manager");
      return {
        instructions: ix.initAccessManager(
          ix.accessManager(managerId),
          authority,
          finalAdmin,
          cpiCallers,
          relayers,
        ),
        checks: { managerId, finalAdmin, cpiCallers, relayers },
        verify: async (signature) => {
          const state = await accessState(ctx.wallet, managerId);
          assert.deepEqual(base58(state.admins), [finalAdmin.toBase58()]);
          assert.deepEqual(base58(state.cpiCallers), base58(cpiCallers));
          for (const relayer of relayers)
            assert.ok(state.relayers.some((key) => key.equals(relayer)));
          return {
            accessManagerProgram: managerId.toBase58(),
            admins: base58(state.admins),
            relayers: base58(state.relayers),
            cpiCallers: base58(state.cpiCallers),
            signature,
          };
        },
      };
    }
    case "set-token-metadata": {
      assert.ok(values.uri, "Pass --uri with the reviewed metadata JSON URL");
      const config = await launch(ctx);
      await requireAuthority(ctx, ctx.aktId, "AKT");
      const before = await readTokenMetadata(ctx.connection, config.mint);
      return {
        instructions: [ix.setTokenMetadata(akt, authority, values.uri)],
        checks: { mint: config.mint, previousUri: before?.uri ?? null },
        notes: [
          "Complete the metadata launch gate (docs/token-metadata.md) for this URI first.",
        ],
        verify: async (signature) => {
          const metadata = await readTokenMetadata(ctx.connection, config.mint);
          assert.ok(metadata);
          assert.equal(metadata.uri, values.uri);
          assert.equal(
            metadata.updateAuthority,
            addresses.mintAuthority.toBase58(),
          );
          return {
            mint: config.mint.toBase58(),
            name: metadata.name,
            symbol: metadata.symbol,
            uri: metadata.uri,
            updateAuthority: metadata.updateAuthority,
            signature,
          };
        },
      };
    }
    case "init-client": {
      const clientId = new web3.PublicKey(named ?? clientIdl.address);
      const managerId = pubkey(values["access-manager"], "--access-manager");
      const checkpoint = await checkpointFile(values.checkpoint);
      const [numerator, denominator] = (values["trust-level"] ?? "1/3").split(
        "/",
      );
      const parameters = ix.clientParameters({
        trustLevel: [
          ix.u64(numerator, "trust level"),
          ix.u64(denominator, "trust level"),
        ],
        trustingPeriod: ix.u64(values["trusting-period"], "--trusting-period"),
        unbondingPeriod: ix.u64(
          values["unbonding-period"],
          "--unbonding-period",
        ),
        maxClockDrift: ix.u64(values["max-clock-drift"], "--max-clock-drift"),
      });
      assert.equal(
        await ctx.connection.getAccountInfo(
          pda("client", clientId),
          commitment,
        ),
        null,
        "this light-client deployment is already initialized",
      );
      await requireAuthority(ctx, clientId, "light client");
      const manager = await accessState(ctx.wallet, managerId);
      const expires =
        BigInt(checkpoint.timestamp) / 1_000_000n +
        parameters.trustingPeriod * 1000n;
      assert.ok(
        expires > BigInt(Date.now()),
        "the checkpoint is older than the trusting period",
      );
      const notes = [
        `The trusting period runs from the checkpoint time: without a header update the client expires at ${new Date(Number(expires)).toISOString()}. Start the relayer as soon as this executes.`,
      ];
      if (parameters.trustingPeriod * 3n > parameters.unbondingPeriod * 2n)
        notes.push(
          "The trusting period exceeds two thirds of the unbonding period.",
        );
      return {
        instructions: [
          ix.initClient(
            ix.lightClient(clientId),
            authority,
            authority,
            managerId,
            checkpoint,
            parameters,
          ),
        ],
        checks: {
          clientId,
          accessManager: managerId,
          accessManagerAdmins: manager.admins,
          checkpoint,
          parameters,
          expiresWithoutUpdate: new Date(Number(expires)).toISOString(),
        },
        notes,
        deployments: { ics07_tendermint: clientId },
        verify: async (signature) => {
          const status = await clientStatus(ctx.wallet, clientId);
          assert.equal(status.chainId, checkpoint.chainId);
          assert.equal(status.latestHeight, checkpoint.height);
          return { client: printable(status), signature };
        },
      };
    }
    case "configure-bridge": {
      const client = pubkey(
        named,
        "--program with the light-client deployment",
      );
      assert.ok(values["chain-id"], "Pass --chain-id");
      const chain = values["chain-id"];
      const policy = policyFrom(values);
      await launch(ctx);
      assert.equal(
        await program.account.bridgeConfig.fetchNullable(
          addresses.bridge,
          commitment,
        ),
        null,
        "the bridge is already configured; use repin or set-mint-policy",
      );
      await requireAuthority(ctx, ctx.aktId, "AKT");
      const target = await clientTarget(ctx, client, chain);
      return {
        instructions: [
          ix.configureBridge(
            akt,
            authority,
            client,
            target.latestHeight,
            chain,
            policy,
          ),
        ],
        checks: { chainId: chain, policy, ...target.checks },
        notes: [
          ...target.notes,
          "Minting stays disabled until set-bridge-enabled.",
        ],
        verify: async (signature) => {
          const bridge = await bridgeConfig(ctx);
          assert.ok(bridge.lightClientProgram.equals(client));
          assert.equal(bridge.chainId, chain);
          return {
            lightClientProgram: client.toBase58(),
            chainId: bridge.chainId,
            policy: showPolicy(bridge.policy),
            bridgeEnabled: (await launch(ctx)).bridgeEnabled,
            signature,
          };
        },
      };
    }
    case "set-bridge-enabled": {
      assert.ok(
        values.enabled === "true" || values.enabled === "false",
        "Pass --enabled true or --enabled false",
      );
      const enabled = values.enabled === "true";
      const before = (await launch(ctx)).bridgeEnabled;
      await bridgeConfig(ctx);
      await requireAuthority(ctx, ctx.aktId, "AKT");
      return {
        instructions: [ix.setBridgeEnabled(akt, authority, enabled)],
        checks: { bridgeEnabled: before, requested: enabled },
        notes: enabled
          ? ["Nothing mints until Akash accepts deposits."]
          : ["The pause guardian can disable minting without the time lock."],
        verify: async (signature) => {
          assert.equal((await launch(ctx)).bridgeEnabled, enabled);
          return { bridgeEnabled: enabled, signature };
        },
      };
    }
    case "set-mint-policy": {
      const policy = policyFrom(values);
      const before = await bridgeConfig(ctx);
      await requireAuthority(ctx, ctx.aktId, "AKT");
      return {
        instructions: [ix.setMintPolicy(akt, authority, policy)],
        checks: { previous: showPolicy(before.policy), policy },
        notes: [
          "Raise Solana caps before Akash's; lower Akash's first (docs/production-deployment.md#parameters).",
        ],
        verify: async (signature) => {
          const after = showPolicy((await bridgeConfig(ctx)).policy);
          assert.deepEqual(after, {
            perTransferMax: policy.perTransferMax.toString(),
            dailyMintCap: policy.dailyMintCap.toString(),
            vaults: base58(policy.vaults),
          });
          return {
            previous: showPolicy(before.policy),
            policy: after,
            signature,
          };
        },
      };
    }
    case "set-guardian":
      return setGuardian(pubkey(values.guardian, "--guardian"));
    case "clear-guardian":
      return setGuardian(null);
    case "pause": {
      const guardianSigner = values["guardian-keypair"]
        ? await keypair(values["guardian-keypair"])
        : undefined;
      const guardian = guardianSigner?.publicKey ?? authority;
      assert.equal(
        await guardianKey(ctx),
        guardian.toBase58(),
        "signer is not the configured pause guardian",
      );
      return {
        instructions: [ix.guardianPause(akt, guardian)],
        checks: {
          guardian,
          bridgeEnabled: (await launch(ctx)).bridgeEnabled,
        },
        signers: guardianSigner ? [guardianSigner] : [],
        verify: async (signature) => {
          assert.equal((await launch(ctx)).bridgeEnabled, false);
          return {
            guardian: guardian.toBase58(),
            bridgeEnabled: false,
            signature,
          };
        },
      };
    }
    case "repin": {
      const client = pubkey(
        named,
        "--program with the new light-client deployment",
      );
      const before = await bridgeConfig(ctx);
      const chain = values["chain-id"] ?? before.chainId;
      assert.ok(
        !client.equals(before.lightClientProgram),
        "that deployment is already pinned",
      );
      await requireAuthority(ctx, ctx.aktId, "AKT");
      const target = await clientTarget(ctx, client, chain);
      // The program never reads the pinned client; report it when readable.
      const previous = await clientStatus(
        ctx.wallet,
        before.lightClientProgram,
      ).then(printable, (error) => ({
        program: before.lightClientProgram.toBase58(),
        ...failure(error),
      }));
      return {
        instructions: [
          ix.repinLightClient(
            akt,
            authority,
            client,
            target.latestHeight,
            chain,
          ),
        ],
        checks: { previous, chainId: chain, ...target.checks },
        notes: [
          ...target.notes,
          "The new client needs relayers, or open relaying, to stay active.",
        ],
        verify: async (signature) => {
          const after = await bridgeConfig(ctx);
          assert.ok(after.lightClientProgram.equals(client));
          assert.equal(after.chainId, chain);
          const event = (
            await transactionEvents(ctx.connection, signature, akt)
          ).find((entry) => entry.name === "LightClientRepinned");
          assert.ok(event, "LightClientRepinned event missing");
          const data = event.data as Record<string, string>;
          return {
            previous,
            pinned: await clientStatus(ctx.wallet, client).then(printable),
            event: {
              previousProgram: data.previous_program,
              program: data.program,
              previousChainId: data.previous_chain_id,
              chainId: data.chain_id,
            },
            relaying: await relaying(ctx.wallet, client).then(
              (value) => value.summary,
              failure,
            ),
            signature,
          };
        },
      };
    }
    case "grant-relayer":
    case "revoke-relayer":
      return setRelayerPlan(
        ctx,
        await targetClient(),
        pubkey(values.key, "--key with the relayer's public key"),
        command === "grant-relayer",
      );
    // Open relaying grants RELAYER_ROLE to the client's own program ID.
    case "open-relaying":
    case "close-relaying": {
      const client = await targetClient();
      return setRelayerPlan(ctx, client, client, command === "open-relaying");
    }
    case "upgrade-program": {
      const target = pubkey(
        values.program,
        "--program with the program to upgrade",
      );
      const buffer = pubkey(values.buffer, "--buffer");
      const spill = values.spill ? new web3.PublicKey(values.spill) : authority;
      const info = await requireAuthority(ctx, target, "program");
      const account = await ctx.connection.getAccountInfo(buffer, commitment);
      // Buffer: u32 tag 1, Option<Pubkey> authority, code.
      assert.ok(
        account &&
          account.owner.equals(ix.BPF_LOADER_UPGRADEABLE) &&
          account.data.readUInt32LE(0) === 1,
        "not a loader buffer",
      );
      assert.ok(
        account.data[4] === 1 &&
          new web3.PublicKey(account.data.subarray(5, 37)).equals(authority),
        "the buffer authority must be the program's upgrade authority",
      );
      const code = account.data.subarray(37);
      const sha256 = createHash("sha256").update(code).digest("hex");
      if (values["expect-sha256"])
        assert.equal(
          sha256,
          values["expect-sha256"].toLowerCase(),
          "buffer contents differ from the published build",
        );
      return {
        instructions: [ix.upgradeProgram(target, buffer, spill, authority)],
        checks: {
          ...info,
          buffer,
          bufferLength: code.length,
          bufferSha256: sha256,
          spill,
        },
        notes: [
          "Compare bufferSha256 with the published verifiable build before approving.",
        ],
        verify: async (signature) => {
          const data = await ctx.connection.getAccountInfo(
            web3.PublicKey.findProgramAddressSync(
              [target.toBuffer()],
              ix.BPF_LOADER_UPGRADEABLE,
            )[0],
            commitment,
          );
          assert.ok(data);
          const deployed = data.data.subarray(45);
          assert.equal(
            createHash("sha256")
              .update(deployed.subarray(0, code.length))
              .digest("hex"),
            sha256,
          );
          assert.ok(deployed.subarray(code.length).every((b) => b === 0));
          return {
            ...(await upgradeInfo(ctx.connection, target)),
            sha256,
            signature,
          };
        },
      };
    }
    default:
      throw new Error(`unknown command ${command}; see local/bridge-admin.ts`);
  }
}

async function show(ctx: Context) {
  const { program, addresses, values } = ctx;
  const named = values.program ?? values["client-program"];
  const config = await program.account.launchConfig.fetchNullable(
    addresses.config,
    commitment,
  );
  const pinned = await program.account.bridgeConfig.fetchNullable(
    addresses.bridge,
    commitment,
  );
  const client =
    pinned?.lightClientProgram ?? (named ? new web3.PublicKey(named) : null);
  // A broken deployment must not hide the rest of the state.
  const relay = client
    ? await relaying(ctx.wallet, client).then((value) => value.summary, failure)
    : null;
  const managerId =
    values["access-manager"] ??
    (relay && "accessManagerProgram" in relay
      ? relay.accessManagerProgram
      : null);
  return {
    genesis: ctx.genesis,
    aktProgram: ctx.aktId.toBase58(),
    bridgeEnabled: config?.bridgeEnabled ?? null,
    mint: config?.mint.toBase58() ?? null,
    chainId: pinned?.chainId ?? null,
    policy: pinned ? showPolicy(pinned.policy) : null,
    mintDay: pinned?.mintDay.toString() ?? null,
    mintedToday: pinned?.mintedToday.toString() ?? null,
    guardian: await guardianKey(ctx),
    lightClientProgram: client?.toBase58() ?? null,
    client: client
      ? await clientStatus(ctx.wallet, client).then(printable, failure)
      : null,
    relaying: relay,
    token: config ? await token(ctx).catch(failure) : null,
    metadata: config
      ? await readTokenMetadata(ctx.connection, config.mint).then(
          (metadata) =>
            metadata && {
              name: metadata.name,
              symbol: metadata.symbol,
              uri: metadata.uri,
              updateAuthority: metadata.updateAuthority,
            },
          failure,
        )
      : null,
    accessManager:
      values["access-manager"] && managerId
        ? await accessState(ctx.wallet, new web3.PublicKey(managerId)).then(
            (state) => ({
              program: state.program.toBase58(),
              admins: base58(state.admins),
              relayers: base58(state.relayers),
              cpiCallers: base58(state.cpiCallers),
            }),
            failure,
          )
        : undefined,
    upgradeAuthorities: {
      akt: await upgradeInfo(ctx.connection, ctx.aktId).catch(failure),
      lightClient: client
        ? await upgradeInfo(ctx.connection, client).catch(failure)
        : null,
      accessManager: managerId
        ? await upgradeInfo(
            ctx.connection,
            new web3.PublicKey(managerId),
          ).catch(failure)
        : null,
    },
  };
}

async function fetchCheckpoint(values: Values) {
  const height = ix.u64(values.height, "--height");
  const rpcs = values["akash-rpc"] ?? [];
  assert.ok(rpcs.length > 0, "Pass --akash-rpc for each independent node");
  assert.ok(values.out, "Pass --out for the checkpoint file");
  if (rpcs.length < 3)
    console.error(
      "warning: the launch ceremony compares at least three independent nodes",
    );
  const observations = [];
  for (const rpc of rpcs) {
    const url = new URL("commit", rpc.endsWith("/") ? rpc : `${rpc}/`);
    url.searchParams.set("height", height.toString());
    const response = await fetch(url, { signal: AbortSignal.timeout(20_000) });
    assert.ok(response.ok, `${rpc}: HTTP ${response.status}`);
    const checkpoint = ix.checkpointFromCommit(await response.json());
    assert.equal(checkpoint.height, height.toString(), `${rpc}: wrong height`);
    observations.push({ rpc, fetchedAt: new Date().toISOString(), checkpoint });
  }
  const agreed = ix.agreeCheckpoint(observations);
  await create(values.out, agreed);
  return agreed;
}

// AKT events of a finalized transaction, from every invocation frame: a
// multisig's execution emits them inside a CPI.
async function transactionEvents(
  connection: web3.Connection,
  signature: string,
  akt: ix.IdlProgram,
) {
  const transaction = await connection.getTransaction(signature, {
    commitment,
    maxSupportedTransactionVersion: 1,
  });
  assert.ok(transaction?.meta, `transaction ${signature} not found`);
  assert.equal(transaction.meta.err, null, `transaction ${signature} failed`);
  return ix.decodeEvents(transaction.meta.logMessages ?? [], akt);
}

async function send(
  connection: web3.Connection,
  instructions: web3.TransactionInstruction[],
  signers: web3.Keypair[],
) {
  return web3.sendAndConfirmTransaction(
    connection,
    new web3.Transaction().add(...instructions),
    signers,
    confirm,
  );
}

async function main() {
  const { positionals, values } = options();
  const [command, ...inputs] = positionals;
  assert.ok(command, "Pass a command; see local/bridge-admin.ts");

  // Offline commands.
  if (command === "checkpoint") {
    console.log(JSON.stringify(await fetchCheckpoint(values), null, 2));
    return;
  }
  if (command === "bundle") {
    assert.ok(values.emit, "Pass --emit for the bundled file");
    const files = await Promise.all(
      inputs.map(async (path) => (await json(path)) as ix.EmitFile),
    );
    const bundle = ix.bundleFiles(files);
    await create(values.emit, bundle);
    console.log(ix.renderSummary(bundle));
    console.log(
      `\nWrote ${bundle.instructions.length} instructions to ${values.emit}`,
    );
    return;
  }
  if (command === "decode") {
    assert.equal(inputs.length, 1, "Pass one emitted file");
    const { file } = ix.readEmitFile(await json(inputs[0]));
    console.log(ix.renderSummary(file));
    console.log(`\nChecks: ${JSON.stringify(file.checks, null, 2)}`);
    return;
  }

  assert.ok(
    values.genesis,
    "Pass --genesis with the verified Solana genesis hash",
  );
  const connection = new web3.Connection(values.url ?? SOLANA_RPC, commitment);
  const genesis = await connection.getGenesisHash();
  assert.equal(
    genesis,
    values.genesis,
    "connected Solana cluster has a different genesis hash",
  );
  const mainnet = genesis === ix.MAINNET_GENESIS;
  const readOnlyCommand = ["show", "relayers", "events"].includes(command);
  const emit = values.emit !== undefined;
  let signer: web3.Keypair | undefined;
  let authority: web3.PublicKey;
  if (emit) {
    assert.ok(!values.keypair, "--emit signs nothing; omit --keypair");
    authority = pubkey(values.authority, "--authority with the multisig vault");
  } else if (readOnlyCommand) {
    authority = web3.PublicKey.default;
  } else {
    if (mainnet)
      assert.ok(
        ["initialize", "init-access-manager", "pause"].includes(command) &&
          values.keypair,
        "Mainnet admin changes go through the multisig: pass --emit FILE --authority VAULT",
      );
    signer = await keypair(values.keypair ?? join(ROOT, "solana-keypair.json"));
    authority = signer.publicKey;
  }
  const wallet = new AnchorProvider(connection, readOnly(authority), confirm);

  if (command === "execute") {
    assert.ok(!mainnet, "execute is for rehearsals; mainnet uses the multisig");
    assert.ok(
      signer && values.keypair,
      "Pass --keypair with the stand-in vault key",
    );
    assert.equal(inputs.length, 1, "Pass one emitted file");
    const {
      file,
      authority: named,
      instructions,
    } = ix.readEmitFile(await json(inputs[0]));
    assert.equal(file.genesisHash, genesis, "the file names another cluster");
    assert.ok(
      named.equals(signer.publicKey),
      "the keypair is not the file's authority",
    );
    // With --cpi-proxy, each instruction arrives by CPI, as from a multisig.
    const proxy = values["cpi-proxy"]
      ? new web3.PublicKey(values["cpi-proxy"])
      : undefined;
    const signature = await send(
      connection,
      proxy
        ? instructions.map((entry) =>
            ix.viaCpiProxy(proxy, entry, signer.publicKey),
          )
        : instructions,
      [signer],
    );
    const aktId = new web3.PublicKey(
      file.summary.find((entry) => entry.program === "akt_solana")?.programId ??
        values["akt-program"] ??
        PROGRAM_ID,
    );
    console.log(
      JSON.stringify(
        {
          command: file.command,
          executed: instructions.length,
          viaCpi: proxy?.toBase58() ?? null,
          signature,
          events: await transactionEvents(connection, signature, ix.akt(aktId)),
        },
        null,
        2,
      ),
    );
    return;
  }

  const aktId = new web3.PublicKey(values["akt-program"] ?? PROGRAM_ID);
  const ctx: Context = {
    values,
    genesis,
    emit,
    authority,
    signer,
    wallet,
    connection,
    aktId,
    akt: ix.akt(aktId),
    program: new Program<AktSolana>(
      { ...idl, address: aktId.toBase58() },
      wallet,
    ),
    addresses: ix.aktAddresses(aktId),
  };

  if (command === "show") {
    console.log(JSON.stringify(await show(ctx), null, 2));
    return;
  }
  // Verifies a multisig execution: the AKT events it emitted, at any depth.
  if (command === "events") {
    assert.ok(
      values.signature,
      "Pass --signature with the executed transaction",
    );
    console.log(
      JSON.stringify(
        {
          signature: values.signature,
          events: await transactionEvents(
            connection,
            values.signature,
            ctx.akt,
          ),
        },
        null,
        2,
      ),
    );
    return;
  }
  if (command === "relayers") {
    const named = values.program ?? values["client-program"];
    const client = named
      ? new web3.PublicKey(named)
      : (await bridgeConfig(ctx)).lightClientProgram;
    console.log(
      JSON.stringify((await relaying(wallet, client)).summary, null, 2),
    );
    return;
  }

  const result = await plan(command, ctx);
  if (result.instructions.length === 0) {
    console.log(
      JSON.stringify(ix.plain({ ...result.checks, changed: false }), null, 2),
    );
    return;
  }
  if (emit) {
    const file = ix.emitFile({
      command,
      genesisHash: genesis,
      authority,
      instructions: result.instructions,
      programs: { akt_solana: aktId, ...result.deployments },
      checks: result.checks,
      notes: result.notes,
    });
    await create(values.emit!, file);
    console.log(ix.renderSummary(file));
    console.log(
      `\nWrote ${file.instructions.length} instruction(s) to ${values.emit}`,
    );
    return;
  }
  assert.ok(signer);
  const signature = await send(connection, result.instructions, [
    signer,
    ...(result.signers ?? []),
  ]);
  const output = {
    ...(result.verify ? await result.verify(signature) : { signature }),
    events: await transactionEvents(connection, signature, ctx.akt),
  };
  console.log(JSON.stringify(ix.plain(output), null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
