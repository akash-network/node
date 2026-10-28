import assert from "node:assert/strict";
import { AnchorProvider, Program, web3 } from "@coral-xyz/anchor";
import idl from "../target/idl/akt_solana.json";
import clientIdl from "../idls/ics07_tendermint.json";
import accessIdl from "../idls/access_manager.json";
import type { Ics07Tendermint } from "../.deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import type { AccessManager } from "../.deps/ibc-contracts/ibc-solana/target/types/access_manager";
import { ACCESS_ID, CLIENT_ID, LE64, PROGRAM_ID, pda } from "./common";

// The bridge account names the light client the AKT program trusts, and
// governance can re-pin it at any time. Runtime tools read it every pass
// instead of using the program IDs a fresh local network deploys.

export interface LightClientIds {
  program: web3.PublicKey; // ICS07 program
  accessManager: web3.PublicKey; // access-manager program
}

// Only for bootstrapping a fresh local network and isolated test validators.
export const LOCAL_LIGHT_CLIENT: LightClientIds = {
  program: CLIENT_ID,
  accessManager: ACCESS_ID,
};

export function lightClientAccounts(ids: LightClientIds) {
  return {
    clientState: pda("client", ids.program),
    appState: pda("app_state", ids.program),
    accessManagerState: pda("access_manager", ids.accessManager),
    consensusAt: (height: bigint) =>
      pda("consensus_state", ids.program, LE64(height)),
  };
}

const discriminator = (
  accounts: { name: string; discriminator: number[] }[],
  name: string,
) => {
  const found = accounts.find((account) => account.name === name);
  assert.ok(found, `IDL lacks ${name}`);
  return Buffer.from(found.discriminator);
};
const BRIDGE = discriminator(idl.accounts, "BridgeConfig");
const APP_STATE = discriminator(clientIdl.accounts, "AppState");

// BridgeConfig starts with light_client_program and a Borsh chain_id string.
// Reading only these keeps discovery independent of later fields.
export function decodeBridgePin(data: Buffer) {
  assert.ok(
    data.length >= 44 && data.subarray(0, 8).equals(BRIDGE),
    "not a bridge account",
  );
  const length = data.readUInt32LE(40);
  assert.ok(
    length > 0 && length <= 64 && data.length >= 44 + length,
    "invalid bridge chain ID",
  );
  return {
    program: new web3.PublicKey(data.subarray(8, 40)),
    chainId: data.subarray(44, 44 + length).toString("utf8"),
  };
}

// AppState starts with am_state.access_manager.
export function decodeAppStateAccessManager(data: Buffer) {
  assert.ok(
    data.length >= 40 && data.subarray(0, 8).equals(APP_STATE),
    "not an ICS07 app state account",
  );
  return new web3.PublicKey(data.subarray(8, 40));
}

export interface LightClient extends LightClientIds {
  chainId: string; // the Akash chain the bridge accepts
  slot: number; // context slot of the bridge read
}

export async function discoverLightClient(
  connection: web3.Connection,
  programId = PROGRAM_ID,
  commitment: web3.Commitment = "confirmed",
): Promise<LightClient> {
  const { context, value: bridge } = await connection.getAccountInfoAndContext(
    pda("bridge", programId),
    commitment,
  );
  assert.ok(bridge, "bridge is not configured");
  assert.ok(bridge.owner.equals(programId), "bridge account has wrong owner");
  const pin = decodeBridgePin(bridge.data);
  const app = await connection.getAccountInfo(pda("app_state", pin.program), {
    commitment,
    minContextSlot: context.slot,
  });
  assert.ok(
    app && app.owner.equals(pin.program),
    `light client ${pin.program.toBase58()} has no app state`,
  );
  return {
    ...pin,
    accessManager: decodeAppStateAccessManager(app.data),
    slot: context.slot,
  };
}

export const clientProgram = (
  wallet: AnchorProvider,
  program: web3.PublicKey,
) =>
  new Program<Ics07Tendermint>(
    { ...clientIdl, address: program.toBase58() },
    wallet,
  );
export const accessProgram = (
  wallet: AnchorProvider,
  program: web3.PublicKey,
) =>
  new Program<AccessManager>(
    { ...accessIdl, address: program.toBase58() },
    wallet,
  );

export const RELAYER_ROLE = 1;
export type RelayerAccess = "member" | "open" | "none";

// A member may update, submit misbehaviour and prune. When RELAYER_ROLE lists
// the ICS07 program's own ID (open relaying), any signer may update and
// submit misbehaviour, but pruning still needs a member.
export function relayerAccess(
  roles: { roleId: { toString(): string }; members: web3.PublicKey[] }[],
  payer: web3.PublicKey,
  program: web3.PublicKey,
): RelayerAccess {
  const members = roles
    .filter((role) => role.roleId.toString() === String(RELAYER_ROLE))
    .flatMap((role) => role.members);
  if (members.some((member) => member.equals(payer))) return "member";
  if (members.some((member) => member.equals(program))) return "open";
  return "none";
}

export async function readRelayerAccess(
  wallet: AnchorProvider,
  ids: LightClientIds,
  payer = wallet.publicKey,
) {
  const address = lightClientAccounts(ids).accessManagerState;
  const info = await wallet.connection.getAccountInfo(address, "confirmed");
  assert.ok(
    info && info.owner.equals(ids.accessManager),
    "access manager state unavailable",
  );
  const state = accessProgram(wallet, ids.accessManager).coder.accounts.decode<{
    roles: { roleId: { toString(): string }; members: web3.PublicKey[] }[];
  }>("accessManager", info.data);
  return relayerAccess(state.roles, payer, ids.program);
}

// Why a discovered light client must not be used: the bridge accepts another
// Akash chain, or governance pinned a program other than the operator's pin.
export function lightClientProblem(
  found: { program: web3.PublicKey; chainId: string },
  expected: { chainId: string; pin?: web3.PublicKey },
) {
  if (found.chainId !== expected.chainId)
    return {
      event: "bridge-chain-mismatch",
      message: `bridge accepts ${found.chainId}, configured ${expected.chainId}`,
    };
  if (expected.pin && !found.program.equals(expected.pin))
    return {
      event: "light-client-mismatch",
      message: `bridge light client ${found.program.toBase58()} differs from pinned ${expected.pin.toBase58()}`,
    };
  return undefined;
}
