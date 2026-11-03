import assert from "node:assert/strict";
import { BN, web3 } from "@coral-xyz/anchor";
import clientIdl from "../idls/ics07_tendermint.json";
import { PROGRAM_ID, pda } from "./common";
import {
  accessProgram,
  clientProgram,
  lightClientAccounts,
} from "./light-client";
import type { relayContext } from "./relay";
export const CHAIN_ID = "sink-discovery-test";
export const NOW = Math.floor(Date.now() / 1000);
export const TRUSTED = 100;
export const HEIGHT = TRUSTED + 1;

export class FakeSolana extends web3.Connection {
  readonly accounts = new Map<string, web3.AccountInfo<Buffer>>();
  constructor() {
    super("http://127.0.0.1:9");
  }
  set(address: web3.PublicKey, owner: web3.PublicKey, data: Buffer) {
    this.accounts.set(address.toBase58(), {
      owner,
      data,
      executable: false,
      lamports: 1,
    });
  }
  override async getGenesisHash() {
    return web3.PublicKey.default.toBase58();
  }
  override async getSlot() {
    return 1;
  }
  override async getBlockTime() {
    return NOW;
  }
  override async getAccountInfo(address: web3.PublicKey) {
    return this.accounts.get(address.toBase58()) ?? null;
  }
  override async getAccountInfoAndContext(address: web3.PublicKey) {
    return { context: { slot: 1 }, value: await this.getAccountInfo(address) };
  }
  override async getMultipleAccountsInfo(addresses: web3.PublicKey[]) {
    return Promise.all(
      addresses.map((address) => this.getAccountInfo(address)),
    );
  }
}

// A paused bridge whose relayer has no header rights, so a tick plans every
// relay and logs why each waits without paying for anything.
export async function testBridge(
  connection: FakeSolana,
  { program, wallet }: Awaited<ReturnType<typeof relayContext>>,
  bridgeEnabled = false,
) {
  const ids = {
    program: web3.Keypair.generate().publicKey,
    accessManager: web3.Keypair.generate().publicKey,
  };
  const accounts = lightClientAccounts(ids);
  connection.set(
    pda("bridge", PROGRAM_ID),
    PROGRAM_ID,
    await program.coder.accounts.encode("bridgeConfig", {
      lightClientProgram: ids.program,
      chainId: CHAIN_ID,
      policy: {
        perTransferMax: new BN(1000),
        dailyMintCap: new BN(1000),
        vaults: [],
      },
      mintDay: new BN(0),
      mintedToday: new BN(0),
    }),
  );
  connection.set(
    pda("config", PROGRAM_ID),
    PROGRAM_ID,
    await program.coder.accounts.encode("launchConfig", {
      version: 1,
      initializer: wallet.publicKey,
      mint: pda("mint", PROGRAM_ID),
      bridgeEnabled,
      initializedAtSlot: new BN(0),
      bump: 0,
      mintAuthorityBump: 0,
    }),
  );
  const appState = clientIdl.accounts.find(({ name }) => name === "AppState");
  assert.ok(appState);
  connection.set(
    accounts.appState,
    ids.program,
    Buffer.concat([
      Buffer.from(appState.discriminator),
      ids.accessManager.toBuffer(),
      Buffer.alloc(257),
    ]),
  );
  connection.set(
    accounts.accessManagerState,
    ids.accessManager,
    await accessProgram(wallet, ids.accessManager).coder.accounts.encode(
      "accessManager",
      { roles: [], whitelistedPrograms: [], pendingAuthorityTransfers: [] },
    ),
  );
  const client = clientProgram(wallet, ids.program).coder.accounts;
  connection.set(
    accounts.clientState,
    ids.program,
    await client.encode("clientState", {
      chainId: CHAIN_ID,
      trustLevelNumerator: new BN(1),
      trustLevelDenominator: new BN(3),
      trustingPeriod: new BN(86_400),
      unbondingPeriod: new BN(172_800),
      maxClockDrift: new BN(10),
      frozenHeight: { revisionNumber: new BN(0), revisionHeight: new BN(0) },
      latestHeight: {
        revisionNumber: new BN(0),
        revisionHeight: new BN(TRUSTED),
      },
    }),
  );
  connection.set(
    accounts.consensusAt(BigInt(TRUSTED)),
    ids.program,
    await client.encode("consensusStateStore", {
      height: new BN(TRUSTED),
      consensusState: {
        timestamp: new BN(NOW).mul(new BN(1_000_000_000)),
        root: Array(32).fill(0),
        nextValidatorsHash: Array(32).fill(0),
      },
    }),
  );
  const clock = Buffer.alloc(40);
  clock.writeBigInt64LE(BigInt(NOW), 32);
  connection.set(web3.SYSVAR_CLOCK_PUBKEY, web3.PublicKey.default, clock);
}

export function relayTestConfig(stateDir: string) {
  return {
    akashRpc: "http://127.0.0.1:9",
    akashChainId: CHAIN_ID,
    solanaRpc: "http://127.0.0.1:9",
    programId: PROGRAM_ID,
    payerKeyFile: "/unused",
    stateDir,
    maxLamportsPerTransfer: 1_000_000_000,
    maxHeaderSignatures: 100,
    refreshFraction: 0.5,
    retryMaxMs: 60_000,
    pruneIntervalMs: 3_600_000,
    maxAgeSeconds: 60,
    pollMs: 1000,
    headerEncoder: "/unused",
  };
}
