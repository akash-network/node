import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { cp, readFile, writeFile } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import { join } from "node:path";
import { promisify } from "node:util";
import { fromBech32, toBech32 } from "@cosmjs/encoding";
import { Comet38Client, HttpClient } from "@cosmjs/tendermint-rpc";
import { MsgCreateValidator } from "cosmjs-types/cosmos/staking/v1beta1/tx";
import { PubKey } from "cosmjs-types/cosmos/crypto/ed25519/keys";
import { relayContext } from "./relay";
import { start, stop, processes, running, ready } from "./stack";
import {
  ROOT,
  PROFILE,
  AKASH,
  AKASH_HOME,
  AKASH_RPC,
  AKASH_RPC_PORT,
  AKASH_GRPC_PORT,
  CHAIN,
  cosmosWallet,
  cosmosSigner,
  fee,
  json,
  object,
  save,
  sleep,
} from "./common";

const exec = promisify(execFile);
export const validatorHome = (index: number) =>
  index === 0 ? AKASH_HOME : join(ROOT, `validator${index}`);
export const validatorName = (index: number) =>
  index === 0 ? "akash" : `akash${index}`;
export const validatorRpc = (index: number) =>
  `http://127.0.0.1:${AKASH_RPC_PORT + index * 10}`;
export function comet(index = 0) {
  return Comet38Client.create(new HttpClient(validatorRpc(index), 15_000));
}
const cli = (home: string, args: string[]) =>
  exec(AKASH, [...args, "--home", home], { maxBuffer: 4 * 1024 * 1024 });

async function configure(home: string, index: number, peers: string) {
  const configPath = join(home, "config/config.toml");
  let config = await readFile(configPath, "utf8");
  config = config
    .replace(
      'laddr = "tcp://127.0.0.1:26657"',
      `laddr = "tcp://127.0.0.1:${AKASH_RPC_PORT + index * 10}"`,
    )
    .replace(
      'laddr = "tcp://0.0.0.0:26656"',
      `laddr = "tcp://127.0.0.1:${AKASH_RPC_PORT - 1 + index * 10}"`,
    )
    .replace(/^persistent_peers = .*$/m, `persistent_peers = "${peers}"`)
    .replace(/^allow_duplicate_ip = .*$/m, "allow_duplicate_ip = true")
    .replace(/^addr_book_strict = .*$/m, "addr_book_strict = false")
    .replace(/^pprof_laddr = .*$/m, 'pprof_laddr = ""')
    .replace(/^timeout_commit = .*$/m, 'timeout_commit = "1s"');
  await writeFile(configPath, config);
  const appPath = join(home, "config/app.toml");
  let app = await readFile(appPath, "utf8");
  app = app
    .replace(
      'address = "localhost:9090"',
      `address = "127.0.0.1:${AKASH_GRPC_PORT + index * 10}"`,
    )
    .replace(/(\[api\][\s\S]*?\nenable = )true/, "$1false")
    .replace(/(\[grpc-web\][\s\S]*?\nenable = )true/, "$1false")
    .replace(/^minimum-gas-prices = .*$/m, 'minimum-gas-prices = "0.025uakt"')
    .replace(/^pruning = .*$/m, 'pruning = "nothing"');
  await writeFile(appPath, app);
}

export async function startValidator(index: number) {
  assert.ok(
    PROFILE === "testnetify" &&
      Number.isInteger(index) &&
      index >= 0 &&
      index < 4,
  );
  const home = validatorHome(index);
  await start(
    validatorName(index),
    AKASH,
    [
      "start",
      "--home",
      home,
      "--rpc.laddr",
      `tcp://127.0.0.1:${AKASH_RPC_PORT + index * 10}`,
      "--p2p.laddr",
      `tcp://127.0.0.1:${AKASH_RPC_PORT - 1 + index * 10}`,
      "--grpc.address",
      `127.0.0.1:${AKASH_GRPC_PORT + index * 10}`,
      "--minimum-gas-prices",
      "0.025uakt",
      "--pruning",
      "nothing",
      "--log_level",
      "warn",
      ...(index === 0 && process.env.AKT_TEAM_API === "1"
        ? [
            "--api.enable",
            "--api.address",
            "tcp://127.0.0.1:1317",
            "--grpc.enable",
          ]
        : []),
    ],
    home,
    [
      AKASH_RPC_PORT + index * 10,
      AKASH_RPC_PORT - 1 + index * 10,
      AKASH_GRPC_PORT + index * 10,
    ],
  );
}

async function prepare() {
  const accounts = [];
  for (let i = 0; i < 4; i++) {
    const key = join(ROOT, `validator${i}.key`);
    if (!existsSync(key))
      await writeFile(key, randomBytes(32).toString("hex") + "\n", {
        mode: 0o600,
      });
    const address = (
      await (await cosmosWallet(`validator${i}`)).getAccounts()
    )[0].address;
    accounts.push({
      address,
      balances: [{ denom: "uakt", amount: "100000000000" }],
    });
    const home = validatorHome(i);
    if (!existsSync(join(home, "config/genesis.json")))
      await cli(home, [
        "genesis",
        "init",
        `sink-validator-${i}`,
        "--chain-id",
        CHAIN,
      ]);
    await configure(home, i, "");
  }
  await save(join(ROOT, "testnetify.json"), {
    chain_id: CHAIN,
    accounts,
    gov: {
      voting_params: { voting_period: "10s", expedited_vote_period: "5s" },
    },
    validators: [
      {
        moniker: "sink-validator-0",
        operator: accounts[0].address,
        status: 3,
        home: "akash",
        commission: {
          rate: "0.1",
          max_rate: "0.2",
          max_change_rate: "0.01",
          update_time: new Date().toISOString(),
        },
        min_self_delegation: "1",
        delegations: [
          {
            address: accounts[0].address,
            amount: { denom: "uakt", amount: "10000000000" },
          },
        ],
      },
    ],
  });
}

export async function startNetwork() {
  assert.equal(PROFILE, "testnetify");
  const converted = join(ROOT, "testnetify-completed.json");
  const pending = join(ROOT, "testnetify-started.json");
  if (!existsSync(converted)) {
    if (!existsSync(pending)) {
      await prepare();
      await startValidator(0);
      const node = comet();
      try {
        await ready("akash", async () => {
          assert.ok((await node.status()).syncInfo.latestBlockHeight >= 8);
        });
      } finally {
        node.disconnect();
      }
      const entry = (await processes()).find((entry) => entry.name === "akash");
      assert.ok(entry);
      await stop(entry);
      // Copy only a stopped, task-owned database. Preserve the seed for review.
      const snapshot = join(ROOT, "seed-snapshot");
      assert.ok(
        !existsSync(snapshot),
        "seed snapshot already exists; inspect the interrupted initialization before proceeding",
      );
      await cp(AKASH_HOME, snapshot, {
        recursive: true,
        errorOnExist: true,
        force: false,
      });
      await save(pending, {
        startedAt: new Date().toISOString(),
        source: snapshot,
      });
      await start(
        "testnetify",
        AKASH,
        [
          "testnetify",
          "--home",
          AKASH_HOME,
          "--testnet-rootdir",
          ROOT,
          "--testnet-config",
          join(ROOT, "testnetify.json"),
          "--node",
          AKASH_RPC,
          "--yes",
        ],
        AKASH_HOME,
        [AKASH_RPC_PORT, AKASH_RPC_PORT - 1, AKASH_GRPC_PORT],
      );
    }
    const process = (await processes()).find(
      (entry) => entry.name === "testnetify",
    );
    assert.ok(
      process,
      "testnetify was interrupted before launch; inspect the saved seed; refusing to mutate it again",
    );
    const deadline = Date.now() + 100_000;
    while (await running(process)) {
      assert.ok(
        Date.now() < deadline,
        "testnetify did not complete; inspect testnetify.log",
      );
      await sleep(500);
    }
    const log = await readFile(join(ROOT, "testnetify.log"), "utf8");
    assert.ok(
      log.includes("testnetify completed"),
      "testnetify failed; inspect testnetify.log; its one-shot mutation will not be repeated",
    );
    await save(converted, {
      completedAt: new Date().toISOString(),
      source: join(ROOT, "seed-snapshot"),
      chainId: CHAIN,
    });
  }
  const peers = [];
  for (let i = 0; i < 4; i++)
    peers.push(
      `${(await cli(validatorHome(i), ["tendermint", "show-node-id"])).stdout.trim()}@127.0.0.1:${AKASH_RPC_PORT - 1 + i * 10}`,
    );
  for (let i = 0; i < 4; i++) {
    const home = validatorHome(i);
    const copied = join(home, "copied-fork.json");
    if (i > 0 && !existsSync(copied)) {
      assert.ok(
        !(await processes()).some((entry) => entry.name === validatorName(i)),
        "refusing to overwrite a validator that has been started",
      );
      const statePath = join(home, "data/priv_validator_state.json");
      const signingState = await readFile(statePath);
      await cp(join(AKASH_HOME, "data"), join(home, "data"), {
        recursive: true,
      });
      if (existsSync(join(AKASH_HOME, "wasm")))
        await cp(join(AKASH_HOME, "wasm"), join(home, "wasm"), {
          recursive: true,
        });
      await cp(
        join(AKASH_HOME, "config/genesis.json"),
        join(home, "config/genesis.json"),
      );
      await writeFile(statePath, signingState, { mode: 0o600 });
      await save(copied, {
        source: AKASH_HOME,
        copiedAt: new Date().toISOString(),
      });
    }
    await configure(home, i, peers.filter((_, j) => j !== i).join(","));
  }
  // All database copies finish before any validator starts.
  for (let i = 0; i < 4; i++) await startValidator(i);
  for (let i = 0; i < 4; i++) {
    const node = comet(i);
    try {
      await ready(validatorName(i), async () => {
        assert.equal((await node.status()).nodeInfo.network, CHAIN);
        assert.ok((await node.status()).syncInfo.latestBlockHeight > 8);
      });
    } finally {
      node.disconnect();
    }
  }
}

export async function activateValidators() {
  const relay = await relayContext();
  const node = comet();
  try {
    for (let i = 1; i < 4; i++) {
      const key = object(
        await json(join(validatorHome(i), "config/priv_validator_key.json")),
      );
      const pubkey = object(key.pub_key);
      assert.equal(pubkey.type, "tendermint/PubKeyEd25519");
      assert.ok(typeof pubkey.value === "string");
      const bytes = Buffer.from(pubkey.value, "base64");
      assert.equal(bytes.length, 32);
      const signer = await cosmosSigner(`validator${i}`);
      try {
        const operator = toBech32(
          "akashvaloper",
          fromBech32(signer.address, 90).data,
        );
        let exists = false;
        try {
          await cli(AKASH_HOME, [
            "query",
            "staking",
            "validator",
            operator,
            "--node",
            AKASH_RPC,
            "--output",
            "json",
          ]);
          exists = true;
        } catch (error) {
          assert.ok(
            error instanceof Error &&
              /not found|does not exist/.test(error.message),
            "staking query failed unexpectedly",
          );
        }
        if (exists) continue;
        const pinned = await relay.lightClient();
        const before = await pinned.client.account.clientState.fetch(
          pinned.clientState,
        );
        const tx = await signer.client.signAndBroadcast(
          signer.address,
          [
            {
              typeUrl: "/cosmos.staking.v1beta1.MsgCreateValidator",
              value: MsgCreateValidator.fromPartial({
                description: { moniker: `sink-validator-${i}` },
                commission: {
                  rate: "100000000000000000",
                  maxRate: "200000000000000000",
                  maxChangeRate: "10000000000000000",
                },
                minSelfDelegation: "1",
                delegatorAddress: signer.address,
                validatorAddress: operator,
                pubkey: {
                  typeUrl: "/cosmos.crypto.ed25519.PubKey",
                  value: PubKey.encode({ key: bytes }).finish(),
                },
                value: { denom: "uakt", amount: "10000000000" },
              }),
            },
          ],
          {
            ...fee,
            gas: "1000000",
            amount: [{ denom: "uakt", amount: "25000" }],
          },
          "Local validator-set transition",
        );
        assert.equal(tx.code, 0, `validator activation failed: ${tx.rawLog}`);
        const deadline = Date.now() + 60_000;
        let target = 0;
        while (true) {
          const height = (await node.status()).syncInfo.latestBlockHeight;
          target = height - 1;
          if (
            target >= tx.height + 2 &&
            (await node.validatorsAll(target)).validators.some(
              (v) => v.pubkey && Buffer.from(v.pubkey.data).equals(bytes),
            )
          )
            break;
          assert.ok(
            Date.now() < deadline,
            "new validator did not enter consensus",
          );
          await sleep(500);
        }
        await relay.update(target);
        const result = {
          validator: i,
          transaction: tx.transactionHash,
          height: target,
          trustedBefore: before.latestHeight.revisionHeight.toString(),
          validators: (await node.validatorsAll(target)).validators.length,
        };
        await save(join(ROOT, `validator-transition-${i}.json`), result);
        console.log(
          JSON.stringify({
            event: "authenticated-validator-transition",
            ...result,
          }),
        );
      } finally {
        signer.client.disconnect();
      }
    }
    assert.equal(
      (await node.validatorsAll()).validators.length,
      4,
      "expected four active validators",
    );
  } finally {
    relay.close();
    node.disconnect();
  }
}
