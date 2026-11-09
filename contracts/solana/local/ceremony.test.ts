import assert from "node:assert/strict";
import { test } from "node:test";
import { web3 } from "@coral-xyz/anchor";
import {
  ceremonyBundle,
  handoverInstructions,
  type LaunchManifest,
} from "./ceremony";
import * as ix from "./admin-instructions";
import { MAINNET_GENESIS } from "../shared/networks";

const key = () => web3.Keypair.generate().publicKey;
const ceremony = key();
const vault = key();
const programs = {
  access_manager: key().toBase58(),
  ics07_tendermint: key().toBase58(),
  akt: key().toBase58(),
};
const manifest: LaunchManifest = {
  version: 1,
  sourceCommit: "a".repeat(40),
  buildDirectory: "build",
  buildInfoSha256: "b".repeat(64),
  checksumsSha256: "c".repeat(64),
  solanaGenesisHash: MAINNET_GENESIS,
  ceremonyAuthority: ceremony.toBase58(),
  upgradeAuthority: vault.toBase58(),
  programs,
};
const akt = ix.akt(new web3.PublicKey(programs.akt));
const client = new web3.PublicKey(programs.ics07_tendermint);
const manager = new web3.PublicKey(programs.access_manager);
const checkpoint: ix.Checkpoint = {
  chainId: "akashnet-2",
  height: "123",
  time: "2026-11-09T00:00:00Z",
  timestamp: "1794182400000000000",
  root: "11".repeat(32),
  validatorsHash: "22".repeat(32),
  nextValidatorsHash: "33".repeat(32),
  blockHash: "44".repeat(32),
};

function emitted(instructions: web3.TransactionInstruction[]) {
  return ix.emitFile({
    command: "reviewed setup",
    genesisHash: MAINNET_GENESIS,
    authority: ceremony,
    instructions,
    programs: {
      akt: akt.programId,
      ics07_tendermint: client,
      access_manager: manager,
    },
    checks: {},
  });
}

test("ceremony accepts metadata and a Squads-style PDA guardian while activation is denied", () => {
  const guardian = web3.PublicKey.findProgramAddressSync(
    [vault.toBuffer()],
    key(),
  )[0];
  const file = emitted([
    ix.setTokenMetadata(
      akt,
      ceremony,
      "https://raw.githubusercontent.com/akash-network/net/3c0b4cce5fd3ab197ea6375ad3de6e8417580217/solana/mainnet/akt.json",
    ),
    ix.setPauseGuardian(akt, ceremony, guardian),
  ]);
  assert.equal(ceremonyBundle(file, manifest, MAINNET_GENESIS).phase, "setup");
  assert.throws(
    () =>
      ceremonyBundle(
        emitted([ix.setBridgeEnabled(akt, ceremony, true)]),
        manifest,
        MAINNET_GENESIS,
      ),
    /cannot activate/,
  );
});

test("open-relaying marker is permitted but admin grants require the complete handover", () => {
  const access = ix.accessManager(manager);
  const open = emitted([
    ix.grantRole(access, ceremony, ix.RELAYER_ROLE, client),
  ]);
  assert.equal(ceremonyBundle(open, manifest, MAINNET_GENESIS).phase, "setup");
  const admin = emitted([ix.grantRole(access, ceremony, ix.ADMIN_ROLE, vault)]);
  assert.throws(() => ceremonyBundle(admin, manifest, MAINNET_GENESIS));
});

test("handover includes both admin-role changes and all three upgrade authorities atomically", () => {
  const instructions = handoverInstructions(manifest);
  assert.equal(instructions.length, 5);
  const file = emitted(instructions);
  assert.equal(
    ceremonyBundle(file, manifest, MAINNET_GENESIS).phase,
    "handover",
  );
  for (let index = 0; index < instructions.length; index++)
    assert.throws(() =>
      ceremonyBundle(
        emitted(instructions.filter((_, i) => i !== index)),
        manifest,
        MAINNET_GENESIS,
      ),
    );
  const anotherVault = { ...manifest, upgradeAuthority: key().toBase58() };
  assert.throws(() => ceremonyBundle(file, anotherVault, MAINNET_GENESIS));
  const tx = new web3.Transaction({
    feePayer: ceremony,
    recentBlockhash: key().toBase58(),
  }).add(...instructions);
  assert.ok(tx.serialize({ requireAllSignatures: false }).length <= 1232);
});

test("client initialization and bridge pinning must share one transaction", () => {
  const initialize = ix.initClient(
    ix.lightClient(client),
    ceremony,
    ceremony,
    manager,
    checkpoint,
    {
      trustLevel: [1n, 3n],
      trustingPeriod: 86400n,
      unbondingPeriod: 1814400n,
      maxClockDrift: 10n,
    },
  );
  const bridge = ix.configureBridge(akt, ceremony, client, 123n, "akashnet-2", {
    perTransferMax: 100n,
    dailyMintCap: 1000n,
    vaults: [],
  });
  assert.equal(
    ceremonyBundle(emitted([initialize, bridge]), manifest, MAINNET_GENESIS)
      .phase,
    "setup",
  );
  for (const instructions of [[initialize], [bridge], [bridge, initialize]])
    assert.throws(
      () => ceremonyBundle(emitted(instructions), manifest, MAINNET_GENESIS),
      /together/,
    );
  const unrelated = {
    ...manifest,
    programs: { ...programs, access_manager: key().toBase58() },
  };
  assert.throws(
    () =>
      ceremonyBundle(emitted([initialize, bridge]), unrelated, MAINNET_GENESIS),
    /wrong access manager/,
  );
});

test("ceremony rejects a different network, signer, or program even with a self-consistent summary", () => {
  const file = emitted([ix.setPauseGuardian(akt, ceremony, vault)]);
  assert.throws(
    () => ceremonyBundle(file, manifest, "different-genesis"),
    /another cluster/,
  );
  assert.throws(
    () =>
      ceremonyBundle(
        file,
        { ...manifest, ceremonyAuthority: key().toBase58() },
        MAINNET_GENESIS,
      ),
    /authority differs/,
  );
  assert.throws(
    () =>
      ceremonyBundle(
        file,
        { ...manifest, programs: { ...programs, akt: key().toBase58() } },
        MAINNET_GENESIS,
      ),
    /outside the release/,
  );
});
