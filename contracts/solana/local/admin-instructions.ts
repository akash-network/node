import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { BN, BorshCoder, web3, type Idl } from "@coral-xyz/anchor";
import { TOKEN_PROGRAM_ID } from "@solana/spl-token";
import aktJson from "../target/idl/akt_solana.json";
import clientJson from "../idls/ics07_tendermint.json";
import accessJson from "../idls/access_manager.json";
import { programEvents } from "../reporter/solana";
import { METADATA_PROGRAM_ID, metadataAddress } from "../shared/token-metadata";
import { LE64, pda, programData } from "./common";

// Admin instructions built from the Anchor IDLs, so they follow the programs.
// The same instructions are either signed with a local key (team testnet,
// launch ceremony) or written unsigned for a multisig to execute. Nothing here
// reads the chain or signs.

export const MAINNET_GENESIS = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp";
export const BPF_LOADER_UPGRADEABLE = new web3.PublicKey(
  "BPFLoaderUpgradeab1e11111111111111111111111",
);
export const ADMIN_ROLE = 0n;
// Patched ICS07 lets any signer relay headers and misbehaviour while its own
// program ID holds this role. Pruning still needs a real member.
export const RELAYER_ROLE = 1n;
export const MAX_VAULTS = 16;
const U64_MAX = (1n << 64n) - 1n;

export const IDLS = {
  akt_solana: aktJson as unknown as Idl,
  ics07_tendermint: clientJson as unknown as Idl,
  access_manager: accessJson as unknown as Idl,
} as const;
export type ProgramName = keyof typeof IDLS;

interface IdlAccountNode {
  name: string;
  writable?: boolean;
  signer?: boolean;
  optional?: boolean;
  address?: string;
  accounts?: IdlAccountNode[];
}

export interface FlatAccount {
  name: string;
  writable: boolean;
  signer: boolean;
  address?: string;
}

// Instruction accounts in IDL order. Group members are named group.member.
export function flattenAccounts(
  nodes: readonly IdlAccountNode[],
  prefix = "",
): FlatAccount[] {
  return nodes.flatMap((node) => {
    const name = prefix + node.name;
    if (node.accounts) return flattenAccounts(node.accounts, `${name}.`);
    assert.ok(!node.optional, `optional account ${name} is not supported`);
    return [
      {
        name,
        writable: node.writable === true,
        signer: node.signer === true,
        ...(node.address === undefined ? {} : { address: node.address }),
      },
    ];
  });
}

export type AccountTree = { [name: string]: web3.PublicKey | AccountTree };

function flattenTree(tree: AccountTree, prefix = "", out = new Map()) {
  for (const [key, value] of Object.entries(tree)) {
    if (value instanceof web3.PublicKey) out.set(prefix + key, value);
    else flattenTree(value, `${prefix}${key}.`, out);
  }
  return out as Map<string, web3.PublicKey>;
}

// One program at one address. Account names and arguments use the IDL's
// snake_case spelling.
export class IdlProgram {
  readonly coder: BorshCoder;
  constructor(
    readonly name: ProgramName,
    readonly programId: web3.PublicKey = new web3.PublicKey(IDLS[name].address),
  ) {
    this.coder = new BorshCoder(IDLS[name]);
  }

  private definition(instruction: string) {
    const found = IDLS[this.name].instructions.find(
      (entry) => entry.name === instruction,
    );
    assert.ok(found, `${this.name} has no instruction ${instruction}`);
    return found;
  }

  accounts(instruction: string): FlatAccount[] {
    return flattenAccounts(
      this.definition(instruction).accounts as IdlAccountNode[],
    );
  }

  // The IDL fixes some addresses. One equal to the IDL's own program address
  // means this program, wherever it is deployed.
  expectedAddress(account: FlatAccount) {
    if (account.address === undefined) return undefined;
    return account.address === IDLS[this.name].address
      ? this.programId
      : new web3.PublicKey(account.address);
  }

  instruction(
    instruction: string,
    args: Record<string, unknown>,
    accounts: AccountTree,
  ) {
    const definition = this.definition(instruction);
    assert.deepEqual(
      Object.keys(args).sort(),
      definition.args.map((arg) => arg.name).sort(),
      `${this.name}.${instruction}: arguments differ from the IDL`,
    );
    const given = flattenTree(accounts);
    const keys = this.accounts(instruction).map((account) => {
      const pubkey = given.get(account.name);
      assert.ok(pubkey, `${this.name}.${instruction}: missing ${account.name}`);
      given.delete(account.name);
      const expected = this.expectedAddress(account);
      assert.ok(
        !expected || expected.equals(pubkey),
        `${this.name}.${instruction}: ${account.name} must be ${expected?.toBase58()}`,
      );
      return {
        pubkey,
        isSigner: account.signer,
        isWritable: account.writable,
      };
    });
    assert.equal(
      given.size,
      0,
      `${this.name}.${instruction}: unknown accounts ${[...given.keys()].join(", ")}`,
    );
    return new web3.TransactionInstruction({
      programId: this.programId,
      keys,
      data: this.coder.instruction.encode(instruction, args),
    });
  }
}

export const akt = (id?: web3.PublicKey) => new IdlProgram("akt_solana", id);
export const lightClient = (id?: web3.PublicKey) =>
  new IdlProgram("ics07_tendermint", id);
export const accessManager = (id?: web3.PublicKey) =>
  new IdlProgram("access_manager", id);

export function u64(value: unknown, name = "value") {
  const text = typeof value === "bigint" ? value.toString() : value;
  assert.ok(
    typeof text === "string" &&
      /^[0-9]+$/.test(text) &&
      BigInt(text) <= U64_MAX,
    `${name} must be an unsigned 64-bit integer`,
  );
  return BigInt(text);
}
const bn = (value: bigint) => new BN(value.toString());

export function aktAddresses(program: web3.PublicKey) {
  return {
    config: pda("config", program),
    bridge: pda("bridge", program),
    guardian: pda("guardian", program),
    mint: pda("mint", program),
    mintAuthority: pda("mint_authority", program),
    programData: programData(program),
  };
}

export function clientAddresses(program: web3.PublicKey) {
  return {
    clientState: pda("client", program),
    appState: pda("app_state", program),
    consensusAt: (height: bigint) =>
      pda("consensus_state", program, LE64(height)),
    programData: programData(program),
  };
}

export const accessManagerState = (program: web3.PublicKey) =>
  pda("access_manager", program);

const upgradeAuthority = (program: IdlProgram, authority: web3.PublicKey) => ({
  program: program.programId,
  program_data: programData(program.programId),
  authority,
});

export interface MintPolicy {
  perTransferMax: bigint;
  dailyMintCap: bigint;
  vaults: web3.PublicKey[];
}

// Same rule as MintPolicy::validate in the program.
export function mintPolicy(policy: MintPolicy) {
  assert.ok(
    policy.perTransferMax > 0n && policy.perTransferMax <= policy.dailyMintCap,
    "per-transfer maximum must be positive and at most the daily cap",
  );
  assert.ok(policy.dailyMintCap <= U64_MAX, "daily cap exceeds uint64");
  assert.ok(policy.vaults.length <= MAX_VAULTS, `at most ${MAX_VAULTS} vaults`);
  assert.ok(
    !policy.vaults.some((vault) => vault.equals(web3.PublicKey.default)),
    "vaults must not include the zero address",
  );
  assert.equal(
    new Set(policy.vaults.map((vault) => vault.toBase58())).size,
    policy.vaults.length,
    "duplicate vault",
  );
  return {
    per_transfer_max: bn(policy.perTransferMax),
    daily_mint_cap: bn(policy.dailyMintCap),
    vaults: policy.vaults,
  };
}

// The program's limit for the bridge chain ID.
function chainId(value: string) {
  assert.ok(
    value.length > 0 && Buffer.byteLength(value) <= 64,
    "chain ID must be 1 to 64 bytes",
  );
  return value;
}

// Genesis creates the launch record and the mint with zero supply.
export function initialize(program: IdlProgram, authority: web3.PublicKey) {
  const { config, mint, mintAuthority } = aktAddresses(program.programId);
  return program.instruction(
    "initialize",
    {},
    {
      program: program.programId,
      program_data: programData(program.programId),
      authority,
      config,
      mint,
      mint_authority: mintAuthority,
      token_program: TOKEN_PROGRAM_ID,
      system_program: web3.SystemProgram.programId,
    },
  );
}

// Same rule as token_metadata::set in the program.
export function metadataUri(uri: string) {
  assert.ok(
    uri.startsWith("https://") &&
      uri.length > 8 &&
      uri.length <= 200 &&
      /^[\x21-\x7e]+$/.test(uri),
    "URI must be HTTPS, printable ASCII and at most 200 bytes",
  );
  return uri;
}

export function setTokenMetadata(
  program: IdlProgram,
  authority: web3.PublicKey,
  uri: string,
) {
  const { config, mint, mintAuthority } = aktAddresses(program.programId);
  return program.instruction(
    "set_token_metadata",
    { uri: metadataUri(uri) },
    {
      admin: upgradeAuthority(program, authority),
      config,
      mint,
      mint_authority: mintAuthority,
      metadata: metadataAddress(mint),
      metadata_program: METADATA_PROGRAM_ID,
      system_program: web3.SystemProgram.programId,
      rent: web3.SYSVAR_RENT_PUBKEY,
    },
  );
}

// configure_bridge and repin_light_client name the client's consensus state at
// its latest height when they execute. A multisig must therefore execute them
// before the client is updated again, or together with the client's
// initialize in one transaction.
function clientAccounts(client: web3.PublicKey, latestHeight: bigint) {
  const addresses = clientAddresses(client);
  return {
    light_client_program: client,
    client_state: addresses.clientState,
    latest_consensus: addresses.consensusAt(latestHeight),
  };
}

export function configureBridge(
  program: IdlProgram,
  authority: web3.PublicKey,
  client: web3.PublicKey,
  latestHeight: bigint,
  chain: string,
  policy: MintPolicy,
) {
  const { config, bridge } = aktAddresses(program.programId);
  return program.instruction(
    "configure_bridge",
    { chain_id: chainId(chain), policy: mintPolicy(policy) },
    {
      admin: upgradeAuthority(program, authority),
      config,
      bridge,
      ...clientAccounts(client, latestHeight),
      system_program: web3.SystemProgram.programId,
    },
  );
}

export function setBridgeEnabled(
  program: IdlProgram,
  authority: web3.PublicKey,
  enabled: boolean,
) {
  const { config, bridge } = aktAddresses(program.programId);
  return program.instruction(
    "set_bridge_enabled",
    { enabled },
    { admin: upgradeAuthority(program, authority), config, bridge },
  );
}

export function setMintPolicy(
  program: IdlProgram,
  authority: web3.PublicKey,
  policy: MintPolicy,
) {
  const { config, bridge } = aktAddresses(program.programId);
  return program.instruction(
    "set_mint_policy",
    { policy: mintPolicy(policy) },
    { admin: upgradeAuthority(program, authority), config, bridge },
  );
}

export function setPauseGuardian(
  program: IdlProgram,
  authority: web3.PublicKey,
  guardian: web3.PublicKey | null,
) {
  return program.instruction(
    "set_pause_guardian",
    { guardian },
    {
      admin: upgradeAuthority(program, authority),
      pause_guardian: aktAddresses(program.programId).guardian,
      system_program: web3.SystemProgram.programId,
    },
  );
}

export function guardianPause(program: IdlProgram, guardian: web3.PublicKey) {
  const { guardian: pauseGuardian, config } = aktAddresses(program.programId);
  return program.instruction(
    "guardian_pause",
    {},
    { guardian, pause_guardian: pauseGuardian, config },
  );
}

export function repinLightClient(
  program: IdlProgram,
  authority: web3.PublicKey,
  client: web3.PublicKey,
  latestHeight: bigint,
  chain: string,
) {
  return program.instruction(
    "repin_light_client",
    { chain_id: chainId(chain) },
    {
      admin: upgradeAuthority(program, authority),
      bridge: aktAddresses(program.programId).bridge,
      ...clientAccounts(client, latestHeight),
    },
  );
}

function roleChange(
  instruction: "grant_role" | "revoke_role",
  program: IdlProgram,
  admin: web3.PublicKey,
  role: bigint,
  account: web3.PublicKey,
) {
  return program.instruction(
    instruction,
    { role_id: bn(role), account },
    {
      access_manager: accessManagerState(program.programId),
      admin,
      instructions_sysvar: web3.SYSVAR_INSTRUCTIONS_PUBKEY,
    },
  );
}

export const grantRole = (
  program: IdlProgram,
  admin: web3.PublicKey,
  role: bigint,
  account: web3.PublicKey,
) => roleChange("grant_role", program, admin, role, account);

export const revokeRole = (
  program: IdlProgram,
  admin: web3.PublicKey,
  role: bigint,
  account: web3.PublicKey,
) => roleChange("revoke_role", program, admin, role, account);

// The access manager's initialize rejects CPI, so a multisig cannot run it.
// Its upgrade authority (the ceremony key) initializes it as the first admin,
// allows the multisig program to call admin instructions by CPI, hands
// ADMIN_ROLE to the multisig vault and renounces its own, in one transaction.
export function initAccessManager(
  program: IdlProgram,
  signer: web3.PublicKey,
  admin: web3.PublicKey,
  cpiCallers: web3.PublicKey[],
  relayers: web3.PublicKey[] = [],
) {
  const state = accessManagerState(program.programId);
  const sysvar = web3.SYSVAR_INSTRUCTIONS_PUBKEY;
  const instructions = [
    program.instruction(
      "initialize",
      { admin: signer },
      {
        access_manager: state,
        payer: signer,
        system_program: web3.SystemProgram.programId,
        instructions_sysvar: sysvar,
        program_data: programData(program.programId),
        authority: signer,
      },
    ),
  ];
  if (cpiCallers.length > 0)
    instructions.push(
      program.instruction(
        "set_whitelisted_programs",
        { whitelisted_programs: cpiCallers },
        { access_manager: state, admin: signer, instructions_sysvar: sysvar },
      ),
    );
  for (const relayer of relayers)
    instructions.push(grantRole(program, signer, RELAYER_ROLE, relayer));
  if (!admin.equals(signer))
    instructions.push(
      grantRole(program, signer, ADMIN_ROLE, admin),
      program.instruction(
        "renounce_role",
        { role_id: bn(ADMIN_ROLE) },
        { access_manager: state, caller: signer, instructions_sysvar: sysvar },
      ),
    );
  return instructions;
}

export interface ClientParameters {
  trustLevel: [bigint, bigint];
  trustingPeriod: bigint;
  unbondingPeriod: bigint;
  maxClockDrift: bigint;
}

// Same checks as the verifier's initialize.
export function clientParameters(value: ClientParameters) {
  const [numerator, denominator] = value.trustLevel;
  assert.ok(
    numerator > 0n && denominator > 0n && numerator <= denominator,
    "trust level must be a fraction in (0, 1]",
  );
  assert.ok(
    value.trustingPeriod > 0n && value.trustingPeriod < value.unbondingPeriod,
    "trusting period must be positive and below the unbonding period",
  );
  assert.ok(value.maxClockDrift > 0n, "max clock drift must be positive");
  return value;
}

export function initClient(
  program: IdlProgram,
  authority: web3.PublicKey,
  payer: web3.PublicKey,
  manager: web3.PublicKey,
  checkpoint: Checkpoint,
  parameters: ClientParameters,
) {
  const { trustLevel, trustingPeriod, unbondingPeriod, maxClockDrift } =
    clientParameters(parameters);
  const height = u64(checkpoint.height, "checkpoint height");
  assert.ok(height > 0n, "checkpoint height must be positive");
  assert.ok(!manager.equals(web3.PublicKey.default), "access manager required");
  const addresses = clientAddresses(program.programId);
  return program.instruction(
    "initialize",
    {
      client_state: {
        chain_id: checkpoint.chainId,
        trust_level_numerator: bn(trustLevel[0]),
        trust_level_denominator: bn(trustLevel[1]),
        trusting_period: bn(trustingPeriod),
        unbonding_period: bn(unbondingPeriod),
        max_clock_drift: bn(maxClockDrift),
        frozen_height: { revision_number: bn(0n), revision_height: bn(0n) },
        latest_height: {
          revision_number: bn(revisionNumber(checkpoint.chainId)),
          revision_height: bn(height),
        },
      },
      consensus_state: {
        timestamp: bn(u64(checkpoint.timestamp, "checkpoint timestamp")),
        root: [...bytes32(checkpoint.root, "root")],
        next_validators_hash: [
          ...bytes32(checkpoint.nextValidatorsHash, "nextValidatorsHash"),
        ],
      },
      access_manager: manager,
    },
    {
      client_state_account: addresses.clientState,
      consensus_state_store: addresses.consensusAt(height),
      app_state: addresses.appState,
      payer,
      system_program: web3.SystemProgram.programId,
      program_data: addresses.programData,
      authority,
    },
  );
}

const UPGRADE_ACCOUNTS: FlatAccount[] = [
  { name: "program_data", writable: true, signer: false },
  { name: "program", writable: true, signer: false },
  { name: "buffer", writable: true, signer: false },
  { name: "spill", writable: true, signer: false },
  { name: "rent", writable: false, signer: false },
  { name: "clock", writable: false, signer: false },
  { name: "authority", writable: false, signer: true },
];

// BPF upgradeable loader Upgrade (instruction 3): replaces the program with a
// buffer whose authority is the program's upgrade authority. The buffer's
// rent goes to spill.
export function upgradeProgram(
  program: web3.PublicKey,
  buffer: web3.PublicKey,
  spill: web3.PublicKey,
  authority: web3.PublicKey,
) {
  const keys = [
    programData(program),
    program,
    buffer,
    spill,
    web3.SYSVAR_RENT_PUBKEY,
    web3.SYSVAR_CLOCK_PUBKEY,
    authority,
  ];
  return new web3.TransactionInstruction({
    programId: BPF_LOADER_UPGRADEABLE,
    keys: UPGRADE_ACCOUNTS.map((account, index) => ({
      pubkey: keys[index],
      isSigner: account.signer,
      isWritable: account.writable,
    })),
    data: Buffer.from([3, 0, 0, 0]),
  });
}

// Rehearsals only: wraps an instruction in upstream's test-cpi-proxy
// proxy_cpi(instruction_data, account_metas), so it arrives by CPI the way a
// multisig executes it. The proxy forwards the outer transaction's signers.
export function viaCpiProxy(
  proxy: web3.PublicKey,
  ix: web3.TransactionInstruction,
  payer: web3.PublicKey,
) {
  const u32 = (value: number) => {
    const bytes = Buffer.alloc(4);
    bytes.writeUInt32LE(value);
    return bytes;
  };
  return new web3.TransactionInstruction({
    programId: proxy,
    keys: [
      { pubkey: ix.programId, isSigner: false, isWritable: false },
      { pubkey: payer, isSigner: true, isWritable: true },
      ...ix.keys,
    ],
    data: Buffer.concat([
      createHash("sha256").update("global:proxy_cpi").digest().subarray(0, 8),
      u32(ix.data.length),
      ix.data,
      u32(ix.keys.length),
      ...ix.keys.map((key) =>
        Buffer.from([Number(key.isSigner), Number(key.isWritable)]),
      ),
    ]),
  });
}

// JSON-friendly copies of decoded values: integers as decimal strings,
// addresses in base58 and byte arrays in hex.
export function plain(value: unknown): unknown {
  if (value === null || value === undefined) return null;
  if (BN.isBN(value)) return value.toString();
  if (typeof value === "bigint") return value.toString();
  if (value instanceof web3.PublicKey) return value.toBase58();
  if (value instanceof Uint8Array) return Buffer.from(value).toString("hex");
  if (Array.isArray(value)) {
    if (value.length > 0 && value.every((v) => Number.isInteger(v)))
      return Buffer.from(value).toString("hex");
    return value.map(plain);
  }
  if (typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, plain(entry)]),
    );
  return value;
}

export interface DecodedEvent {
  name: string;
  data: unknown;
}

// A multisig executes admin instructions by CPI, and Anchor's EventParser
// ignores events emitted inside a CPI. Attribute each `Program data:` line to
// its invocation frame instead, with the reporters' parser, and keep the
// events this program emitted at any depth.
export function decodeEvents(
  logs: readonly string[],
  program: IdlProgram,
): DecodedEvent[] {
  return programEvents(logs, program.programId, (data) =>
    program.coder.events.decode(data),
  ).map((event) => ({ name: event.name, data: plain(event.data) }));
}

export interface DecodedAccount extends FlatAccount {
  pubkey: string;
}

export interface DecodedInstruction {
  program: string;
  programId: string;
  instruction: string;
  args: unknown;
  accounts: DecodedAccount[];
}

// Decodes an instruction with the IDL of the named program, and fails unless
// its accounts, signer and writable flags and fixed addresses match the IDL.
export function decodeInstruction(
  ix: web3.TransactionInstruction,
  name: ProgramName | "bpf_loader_upgradeable",
): DecodedInstruction {
  let instruction: string;
  let args: unknown;
  let expected: FlatAccount[];
  let program: IdlProgram | undefined;
  if (name === "bpf_loader_upgradeable") {
    assert.ok(ix.programId.equals(BPF_LOADER_UPGRADEABLE), "not the loader");
    assert.ok(
      ix.data.equals(Buffer.from([3, 0, 0, 0])),
      "only the loader's Upgrade instruction is supported",
    );
    instruction = "upgrade";
    args = {};
    expected = UPGRADE_ACCOUNTS.map((account, index) =>
      index === 4
        ? { ...account, address: web3.SYSVAR_RENT_PUBKEY.toBase58() }
        : index === 5
          ? { ...account, address: web3.SYSVAR_CLOCK_PUBKEY.toBase58() }
          : account,
    );
    assert.ok(
      ix.keys.length === expected.length &&
        programData(ix.keys[1].pubkey).equals(ix.keys[0].pubkey),
      "program_data does not belong to program",
    );
  } else {
    program = new IdlProgram(name, ix.programId);
    let decoded: ReturnType<typeof program.coder.instruction.decode>;
    try {
      decoded = program.coder.instruction.decode(ix.data);
    } catch (error) {
      throw new Error(`data does not decode as ${name}: ${String(error)}`);
    }
    assert.ok(decoded, `data is not a ${name} instruction`);
    // Canonical bytes only: nothing the summary would not show.
    assert.ok(
      program.coder.instruction
        .encode(decoded.name, decoded.data)
        .equals(ix.data),
      `${decoded.name}: data has bytes the IDL does not describe`,
    );
    instruction = decoded.name;
    args = plain(decoded.data);
    expected = program.accounts(instruction);
  }
  assert.equal(
    ix.keys.length,
    expected.length,
    `${name}.${instruction}: account count differs from the IDL`,
  );
  const accounts = expected.map((account, index) => {
    const key = ix.keys[index];
    assert.equal(
      key.isSigner,
      account.signer,
      `${instruction}: ${account.name} signer flag`,
    );
    assert.equal(
      key.isWritable,
      account.writable,
      `${instruction}: ${account.name} writable flag`,
    );
    const fixed = program
      ? program.expectedAddress(account)
      : account.address === undefined
        ? undefined
        : new web3.PublicKey(account.address);
    assert.ok(
      !fixed || fixed.equals(key.pubkey),
      `${instruction}: ${account.name} must be ${fixed?.toBase58()}`,
    );
    return { ...account, pubkey: key.pubkey.toBase58() };
  });
  return {
    program: name,
    programId: ix.programId.toBase58(),
    instruction,
    args,
    accounts,
  };
}

// The program a known instruction belongs to; only these IDLs are emitted.
export function programName(
  ix: web3.TransactionInstruction,
  programs: Partial<Record<ProgramName, web3.PublicKey>>,
): ProgramName | "bpf_loader_upgradeable" {
  if (ix.programId.equals(BPF_LOADER_UPGRADEABLE))
    return "bpf_loader_upgradeable";
  const found = (Object.keys(IDLS) as ProgramName[]).find((name) =>
    (programs[name] ?? new web3.PublicKey(IDLS[name].address)).equals(
      ix.programId,
    ),
  );
  assert.ok(found, `unknown program ${ix.programId.toBase58()}`);
  return found;
}

export const EMIT_FORMAT = "akt-solana-admin-instructions/v1";

export interface EmittedInstruction {
  programId: string;
  accounts: { pubkey: string; isSigner: boolean; isWritable: boolean }[];
  data: string;
}

export interface EmitFile {
  format: typeof EMIT_FORMAT;
  command: string;
  createdAt: string;
  genesisHash: string;
  authority: string;
  instructions: EmittedInstruction[];
  summary: DecodedInstruction[];
  checks: unknown;
  notes: string[];
}

export function emitFile(params: {
  command: string;
  genesisHash: string;
  authority: web3.PublicKey;
  instructions: web3.TransactionInstruction[];
  programs: Partial<Record<ProgramName, web3.PublicKey>>;
  checks?: unknown;
  notes?: string[];
  createdAt?: Date;
}): EmitFile {
  assert.ok(params.instructions.length > 0, "nothing to emit");
  const authority = params.authority.toBase58();
  const summary = params.instructions.map((ix) =>
    decodeInstruction(ix, programName(ix, params.programs)),
  );
  // Every signer is the authority: the multisig vault signs by CPI.
  for (const ix of params.instructions)
    for (const key of ix.keys)
      if (key.isSigner)
        assert.equal(
          key.pubkey.toBase58(),
          authority,
          "an emitted instruction needs a signer other than the authority",
        );
  return {
    format: EMIT_FORMAT,
    command: params.command,
    createdAt: (params.createdAt ?? new Date()).toISOString(),
    genesisHash: params.genesisHash,
    authority,
    instructions: params.instructions.map((ix) => ({
      programId: ix.programId.toBase58(),
      accounts: ix.keys.map((key) => ({
        pubkey: key.pubkey.toBase58(),
        isSigner: key.isSigner,
        isWritable: key.isWritable,
      })),
      data: ix.data.toString("base64"),
    })),
    summary,
    checks: plain(params.checks ?? {}),
    notes: params.notes ?? [],
  };
}

function strictBase64(value: unknown) {
  assert.ok(
    typeof value === "string" &&
      /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(
        value,
      ),
    "instruction data must be base64",
  );
  return Buffer.from(value, "base64");
}

// Parses an emitted file and checks that its summary is what the instruction
// bytes decode to. Returns the instructions ready to sign.
export function readEmitFile(value: unknown) {
  const file = value as EmitFile;
  assert.ok(
    typeof file === "object" && file !== null && file.format === EMIT_FORMAT,
    `not an ${EMIT_FORMAT} file`,
  );
  assert.ok(
    Array.isArray(file.instructions) &&
      Array.isArray(file.summary) &&
      file.instructions.length === file.summary.length &&
      file.instructions.length > 0,
    "instructions and summary must match one to one",
  );
  const authority = new web3.PublicKey(file.authority);
  const instructions = file.instructions.map((entry) => {
    assert.ok(Array.isArray(entry.accounts), "accounts required");
    return new web3.TransactionInstruction({
      programId: new web3.PublicKey(entry.programId),
      keys: entry.accounts.map((key) => {
        assert.ok(
          typeof key.isSigner === "boolean" &&
            typeof key.isWritable === "boolean",
          "account flags must be booleans",
        );
        assert.ok(
          !key.isSigner || new web3.PublicKey(key.pubkey).equals(authority),
          "only the authority may sign",
        );
        return {
          pubkey: new web3.PublicKey(key.pubkey),
          isSigner: key.isSigner,
          isWritable: key.isWritable,
        };
      }),
      data: strictBase64(entry.data),
    });
  });
  instructions.forEach((ix, index) => {
    const claimed = file.summary[index];
    const name = claimed?.program as ProgramName | "bpf_loader_upgradeable";
    assert.ok(
      name === "bpf_loader_upgradeable" || name in IDLS,
      `instruction ${index + 1}: unknown program ${String(name)}`,
    );
    assert.deepEqual(
      decodeInstruction(ix, name),
      claimed,
      `instruction ${index + 1}: summary does not match its bytes`,
    );
  });
  return { file, authority, instructions };
}

// Concatenates emitted files in order, for one multisig transaction. They
// must target the same cluster and authority.
export function bundleFiles(files: EmitFile[], createdAt = new Date()) {
  assert.ok(files.length > 1, "bundle at least two files");
  for (const file of files) readEmitFile(file);
  const [first] = files;
  for (const file of files) {
    assert.equal(file.genesisHash, first.genesisHash, "different clusters");
    assert.equal(file.authority, first.authority, "different authorities");
  }
  return {
    format: EMIT_FORMAT,
    command: files.map((file) => file.command).join(" + "),
    createdAt: createdAt.toISOString(),
    genesisHash: first.genesisHash,
    authority: first.authority,
    instructions: files.flatMap((file) => file.instructions),
    summary: files.flatMap((file) => file.summary),
    checks: files.map((file) => ({
      command: file.command,
      checks: file.checks,
    })),
    notes: files.flatMap((file) => file.notes),
  } satisfies EmitFile;
}

function inline(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(inline).join(", ")}]`;
  return `{ ${Object.entries(value)
    .map(([key, entry]) => `${key}: ${inline(entry)}`)
    .join(", ")} }`;
}

export function renderSummary(file: EmitFile) {
  const lines = [
    `Command:   ${file.command}`,
    `Cluster:   genesis ${file.genesisHash}`,
    `Authority: ${file.authority} (the only signer)`,
    "",
  ];
  file.summary.forEach((ix, index) => {
    lines.push(
      `${index + 1}. ${ix.program}.${ix.instruction}  program ${ix.programId}`,
    );
    const args = Object.entries((ix.args ?? {}) as Record<string, unknown>);
    for (const [key, value] of args) lines.push(`   ${key} = ${inline(value)}`);
    const width = Math.max(...ix.accounts.map((a) => a.name.length));
    for (const account of ix.accounts) {
      const flags = [
        account.signer ? "signer" : "",
        account.writable ? "writable" : "",
      ]
        .filter(Boolean)
        .join(" ");
      lines.push(
        `   ${account.name.padEnd(width)}  ${account.pubkey}${flags ? `  ${flags}` : ""}`,
      );
    }
    lines.push("");
  });
  for (const note of file.notes) lines.push(`Note: ${note}`);
  return lines.join("\n").trimEnd();
}

// --- Checkpoint ceremony ---------------------------------------------------

export interface Checkpoint {
  chainId: string;
  height: string;
  time: string;
  timestamp: string;
  root: string;
  nextValidatorsHash: string;
  validatorsHash: string;
  blockHash: string;
  sources?: { rpc: string; fetchedAt: string }[];
}

// IBC revision number: the trailing -N of a chain ID such as akashnet-2.
export function revisionNumber(chain: string) {
  const match = /^.+-(0|[1-9][0-9]*)$/.exec(chain);
  return match ? u64(match[1], "revision number") : 0n;
}

// CometBFT RFC 3339 time, to nanoseconds since the epoch, without rounding.
export function rfc3339Nanos(value: string) {
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(
    value,
  );
  assert.ok(match, `not a UTC RFC 3339 time: ${value}`);
  const seconds = Date.parse(`${match[1]}Z`);
  assert.ok(Number.isFinite(seconds), `invalid time: ${value}`);
  return (
    BigInt(seconds / 1000) * 1_000_000_000n +
    BigInt((match[2] ?? "").padEnd(9, "0"))
  );
}

const hash32 = (value: unknown, name: string) => {
  assert.ok(
    typeof value === "string" && /^[0-9a-fA-F]{64}$/.test(value),
    `${name} must be 32 bytes of hex`,
  );
  return value.toLowerCase();
};

function bytes32(value: string, name: string) {
  return Buffer.from(hash32(value, name), "hex");
}

// The fields of a CometBFT /commit result that the client trusts.
export function checkpointFromCommit(response: unknown): Checkpoint {
  const body = response as {
    result?: {
      canonical?: boolean;
      signed_header?: {
        header?: Record<string, unknown>;
        commit?: { height?: string; block_id?: { hash?: string } };
      };
    };
  };
  const result = body?.result;
  assert.ok(result?.canonical === true, "commit is not canonical yet");
  const header = result.signed_header?.header;
  const commit = result.signed_header?.commit;
  assert.ok(header && commit, "signed header missing");
  assert.equal(commit.height, header.height, "commit is for another height");
  assert.ok(typeof header.chain_id === "string", "chain ID missing");
  assert.ok(typeof header.time === "string", "time missing");
  const height = u64(header.height as string, "height");
  return {
    chainId: header.chain_id,
    height: height.toString(),
    time: header.time,
    timestamp: rfc3339Nanos(header.time).toString(),
    root: hash32(header.app_hash, "app_hash"),
    nextValidatorsHash: hash32(
      header.next_validators_hash,
      "next_validators_hash",
    ),
    validatorsHash: hash32(header.validators_hash, "validators_hash"),
    blockHash: hash32(commit.block_id?.hash, "block hash"),
  };
}

const CHECKPOINT_FIELDS = [
  "chainId",
  "height",
  "time",
  "timestamp",
  "root",
  "nextValidatorsHash",
  "validatorsHash",
  "blockHash",
] as const;

// All sources must report the same header. Disagreement stops the ceremony.
export function agreeCheckpoint(
  observations: { rpc: string; fetchedAt: string; checkpoint: Checkpoint }[],
) {
  assert.ok(observations.length > 0, "no checkpoint sources");
  const [first] = observations;
  for (const { rpc, checkpoint } of observations)
    for (const field of CHECKPOINT_FIELDS)
      assert.equal(
        checkpoint[field],
        first.checkpoint[field],
        `${rpc} reports a different ${field}`,
      );
  return {
    ...first.checkpoint,
    sources: observations.map(({ rpc, fetchedAt }) => ({ rpc, fetchedAt })),
  };
}

export function parseCheckpoint(value: unknown): Checkpoint {
  const raw = value as Record<string, unknown>;
  assert.ok(typeof raw === "object" && raw !== null, "checkpoint object");
  assert.ok(
    typeof raw.chainId === "string" && raw.chainId.length > 0,
    "chainId required",
  );
  assert.ok(typeof raw.time === "string", "time required");
  const height = u64(raw.height as string, "height");
  assert.ok(height > 0n, "height must be positive");
  const timestamp = u64(raw.timestamp as string, "timestamp");
  assert.equal(
    timestamp,
    rfc3339Nanos(raw.time),
    "timestamp does not match time",
  );
  return {
    chainId: raw.chainId,
    height: height.toString(),
    time: raw.time,
    timestamp: timestamp.toString(),
    root: hash32(raw.root, "root"),
    nextValidatorsHash: hash32(raw.nextValidatorsHash, "nextValidatorsHash"),
    validatorsHash: hash32(raw.validatorsHash, "validatorsHash"),
    blockHash: hash32(raw.blockHash, "blockHash"),
    ...(Array.isArray(raw.sources)
      ? { sources: raw.sources as Checkpoint["sources"] }
      : {}),
  };
}
