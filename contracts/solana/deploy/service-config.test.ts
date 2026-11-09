import assert from "node:assert/strict";
import { test } from "node:test";
import { web3 } from "@coral-xyz/anchor";
import { toBech32 } from "@cosmjs/encoding";
import { MAINNET_GENESIS } from "../shared/networks";
import { checkServiceConfig } from "./service-config";

const network = {
  akashRpc: "http://127.0.0.1:26657",
  akashChainId: "akashnet-2",
  solanaRpc: "http://127.0.0.1:8899",
  solanaGenesisHash: MAINNET_GENESIS,
  programId: web3.Keypair.generate().publicKey.toBase58(),
};
const file = "/etc/akt-bridge/config.json";
const relayer = {
  ...network,
  payerKeyFile: "/etc/akt-relayer/payer.json",
  stateDir: "/var/lib/akt-relayer",
  headerEncoder: "/opt/akt-bridge/current/bin/akt-header-encoder",
};
const reporter = {
  ...network,
  mint: web3.Keypair.generate().publicKey.toBase58(),
  reporter: toBech32("akash", new Uint8Array(20).fill(1)),
  maxAgeSeconds: 60,
  pollSeconds: 15,
  feeAmountUakt: "12500",
  gasLimit: "500000",
};

test("explicit testnet mode requires both chains to be non-mainnet", () => {
  const config = {
    ...relayer,
    akashChainId: "akash-bridge-v2-testnetify-1",
    solanaGenesisHash: web3.Keypair.generate().publicKey.toBase58(),
    programId: "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
  };
  checkServiceConfig("relayer", config, file, "testnet");
  assert.throws(
    () => checkServiceConfig("relayer", config, file),
    /akashnet-2/,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        { ...config, akashChainId: "akashnet-2" },
        file,
        "testnet",
      ),
    /refuses Akash mainnet/,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        { ...config, solanaGenesisHash: MAINNET_GENESIS },
        file,
        "testnet",
      ),
    /refuses Solana mainnet/,
  );
});

test("production services require explicit network, program and durable paths", () => {
  checkServiceConfig("relayer", relayer, file);
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        { ...relayer, akashChainId: "testnet-1" },
        file,
      ),
    /akashnet-2/,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        {
          ...relayer,
          solanaGenesisHash: web3.Keypair.generate().publicKey.toBase58(),
        },
        file,
      ),
    /mainnet-beta/,
  );
  assert.throws(() =>
    checkServiceConfig(
      "relayer",
      { ...relayer, headerEncoder: "relative" },
      file,
    ),
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        { ...relayer, stateDir: "/opt/release/.cache/state" },
        file,
      ),
    /survive release/,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "relayer",
        {
          ...relayer,
          programId: "FfFAPUKqjf68ggj1HvpyzMHFDRnT2cFAdZYYQb7mK9i5",
        },
        file,
      ),
    /development/,
  );
});

test("reporter preview needs no signer; production broadcast supports file keys or external custody", () => {
  checkServiceConfig("reporter-preview", reporter, file);
  assert.throws(
    () => checkServiceConfig("reporter-broadcast", reporter, file),
    /keyFile or externalSigner/,
  );
  checkServiceConfig(
    "reporter-broadcast",
    {
      ...reporter,
      externalSigner: { command: "/opt/signer/sign", args: [] },
    },
    file,
  );
  checkServiceConfig(
    "reporter-broadcast",
    {
      ...reporter,
      keyFile: "/etc/akt-reporter/reporter.key",
    },
    file,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "reporter-broadcast",
        {
          ...reporter,
          keyFile: "relative.key",
        },
        file,
      ),
    /absolute/,
  );
  assert.throws(
    () =>
      checkServiceConfig(
        "reporter-broadcast",
        {
          ...reporter,
          keyFile: "/etc/akt-reporter/reporter.key",
          externalSigner: { command: "/opt/signer/sign" },
        },
        file,
      ),
    /not both/,
  );
  assert.throws(() => checkServiceConfig("reporter", reporter, file));
});
