import assert from "node:assert/strict";
import { execFile, spawn, type ChildProcess } from "node:child_process";
import { openSync, closeSync } from "node:fs";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { promisify } from "node:util";
import { join, resolve } from "node:path";
import { createHash } from "node:crypto";
import { toBech32 } from "@cosmjs/encoding";
import { Comet38Client, HttpClient } from "@cosmjs/tendermint-rpc";
import {
  QueryBalanceRequest,
  QueryBalanceResponse,
} from "cosmjs-types/cosmos/bank/v1beta1/query";
import {
  BaseAccount,
  ModuleAccount,
} from "cosmjs-types/cosmos/auth/v1beta1/auth";
import {
  QueryAccountRequest,
  QueryAccountResponse,
} from "cosmjs-types/cosmos/auth/v1beta1/query";
import {
  QueryCommunityPoolRequest,
  QueryCommunityPoolResponse,
} from "cosmjs-types/cosmos/distribution/v1beta1/query";
import { QueryStateResponse } from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { REPO, NODE_REPO, AKASH, object, sleep, save } from "./common";
import { governanceAddress } from "./governance";
const exec = promisify(execFile);
const original = resolve(
  process.env.AKASH_UPGRADE_BASE ??
    join(NODE_REPO, ".cache/upgrade-base/.cache/bin/akash"),
);
const rpc = "http://127.0.0.1:30657";
const sink = toBech32(
  "akash",
  createHash("sha256").update("sink").digest().subarray(0, 20),
);
async function main() {
  await mkdir(join(REPO, ".cache/upgrades"), { recursive: true, mode: 0o700 });
  const root = await mkdtemp(join(REPO, ".cache/upgrades/run-"));
  console.log(`Existing-chain upgrade rehearsal: ${root}`);
  const results: object[] = [];
  // The upgrade installs sink with a zero balance: there is no launch
  // reserve to fund, so no plan-info account can halt it.
  for (const scenario of ["fresh", "preexisting-balance"]) {
    const home = join(root, scenario);
    const chain = `akash-sink-upgrade-local-${scenario}-1`;
    const cli = async (binary: string, args: string[]) =>
      exec(binary, [...args, "--home", home], { maxBuffer: 4 * 1024 * 1024 });
    await cli(original, [
      "genesis",
      "init",
      "sink-upgrade",
      "--chain-id",
      chain,
    ]);
    const addresses = new Map<string, string>();
    for (const name of ["validator", "bystander"]) {
      await cli(original, [
        "keys",
        "add",
        name,
        "--keyring-backend",
        "test",
        "--output",
        "json",
      ]); // Never print generated mnemonic.
      const address = (
        await cli(original, [
          "keys",
          "show",
          name,
          "-a",
          "--keyring-backend",
          "test",
        ])
      ).stdout.trim();
      addresses.set(name, address);
      await cli(original, [
        "genesis",
        "add-account",
        address,
        `${name === "validator" ? "100000000000" : "1000000"}uakt`,
      ]);
    }
    if (scenario === "preexisting-balance")
      await cli(original, ["genesis", "add-account", sink, "1uakt"]);
    const genesisPath = join(home, "config/genesis.json");
    const genesis = object(JSON.parse(await readFile(genesisPath, "utf8")));
    const state = object(genesis.app_state);
    assert.equal(state.sink, undefined, "base binary already has sink");
    const gov = object(state.gov);
    gov.params = {
      ...object(gov.params),
      voting_period: "10s",
      expedited_voting_period: "5s",
    };
    state.gov = gov;
    const mint = object(state.mint);
    mint.minter = {
      ...object(mint.minter),
      inflation: "0.000000000000000000",
      annual_provisions: "0.000000000000000000",
    };
    mint.params = {
      ...object(mint.params),
      inflation_min: "0.000000000000000000",
      inflation_max: "0.000000000000000000",
    };
    state.mint = mint;
    genesis.app_state = state;
    await save(genesisPath, genesis);
    await cli(original, [
      "genesis",
      "gentx",
      "validator",
      "10000000000uakt",
      "--chain-id",
      chain,
      "--keyring-backend",
      "test",
      "--ip",
      "127.0.0.1",
      "--min-self-delegation",
      "1",
      "--commission-rate",
      "0.1",
      "--commission-max-rate",
      "0.2",
      "--commission-max-change-rate",
      "0.01",
    ]);
    await cli(original, ["genesis", "collect"]);
    const configPath = join(home, "config/config.toml");
    await writeFile(
      configPath,
      (await readFile(configPath, "utf8"))
        .replace(/^timeout_commit = .*$/m, 'timeout_commit = "1s"')
        .replace(/^pprof_laddr = .*$/m, 'pprof_laddr = ""'),
    );
    let child: ChildProcess | undefined;
    const node = Comet38Client.create(new HttpClient(rpc, 5000));
    const height = async () => (await node.status()).syncInfo.latestBlockHeight;
    async function start(binary: string, phase: string) {
      const log = openSync(join(home, `${phase}.log`), "a", 0o600);
      child = spawn(
        binary,
        [
          "start",
          "--home",
          home,
          "--rpc.laddr",
          "tcp://127.0.0.1:30657",
          "--p2p.laddr",
          "tcp://127.0.0.1:30656",
          "--grpc.address",
          "127.0.0.1:9390",
          "--minimum-gas-prices",
          "0.025uakt",
          "--pruning",
          "nothing",
          "--log_level",
          "info",
        ],
        { stdio: ["ignore", log, log] },
      );
      closeSync(log);
    }
    async function stop() {
      if (!child || child.exitCode !== null) return;
      const owned = child;
      const stopped = new Promise<void>((resolve) =>
        owned.once("exit", () => resolve()),
      );
      owned.kill("SIGTERM");
      const timer = setTimeout(() => owned.kill("SIGKILL"), 10000);
      await stopped;
      clearTimeout(timer);
    }
    async function waitHeight(target: number) {
      const deadline = Date.now() + 60000;
      for (;;) {
        assert.ok(
          child?.exitCode === null,
          `${scenario}: node exited; inspect ${home}`,
        );
        assert.ok(
          Date.now() < deadline,
          `${scenario}: node did not reach ${target}`,
        );
        try {
          if ((await height()) >= target) return;
        } catch {
          /* RPC startup */
        }
        await sleep(300);
      }
    }
    async function bank(address: string) {
      const response = await node.abciQuery({
        path: "/cosmos.bank.v1beta1.Query/Balance",
        data: QueryBalanceRequest.encode({ address, denom: "uakt" }).finish(),
      });
      assert.equal(response.code, 0);
      return QueryBalanceResponse.decode(response.value).balance?.amount ?? "0";
    }
    async function account(address: string) {
      const response = await node.abciQuery({
        path: "/cosmos.auth.v1beta1.Query/Account",
        data: QueryAccountRequest.encode({ address }).finish(),
      });
      assert.equal(response.code, 0, `missing account ${address}`);
      const any = QueryAccountResponse.decode(response.value).account;
      assert.ok(any);
      if (any.typeUrl === "/cosmos.auth.v1beta1.ModuleAccount") {
        const decoded = ModuleAccount.decode(any.value);
        return {
          module: decoded.name,
          number: decoded.baseAccount?.accountNumber,
        };
      }
      assert.equal(any.typeUrl, "/cosmos.auth.v1beta1.BaseAccount");
      return {
        module: "",
        number: BaseAccount.decode(any.value).accountNumber,
      };
    }
    // LegacyDec amounts are encoded as integers scaled by 10^18.
    async function communityPool() {
      const response = await node.abciQuery({
        path: "/cosmos.distribution.v1beta1.Query/CommunityPool",
        data: QueryCommunityPoolRequest.encode({}).finish(),
      });
      assert.equal(response.code, 0);
      const coin = QueryCommunityPoolResponse.decode(response.value).pool.find(
        (item) => item.denom === "uakt",
      );
      return BigInt(coin?.amount ?? "0") / 10n ** 18n;
    }
    async function transact(args: string[]) {
      const out = await cli(original, [
        "tx",
        ...args,
        "--from",
        "validator",
        "--keyring-backend",
        "test",
        "--chain-id",
        chain,
        "--node",
        rpc,
        "--fees",
        "50000uakt",
        "--gas",
        "2000000",
        "--yes",
        "--output",
        "json",
      ]);
      const tx = object(JSON.parse(out.stdout));
      assert.equal(Number(tx.code), 0, String(tx.raw_log));
      assert.ok(typeof tx.txhash === "string");
      const deadline = Date.now() + 15000;
      for (;;) {
        try {
          const included = await node.tx({
            hash: Buffer.from(tx.txhash, "hex"),
          });
          assert.equal(included.result.code, 0, included.result.log);
          return included;
        } catch (error) {
          if (Date.now() > deadline) throw error;
          await sleep(500);
        }
      }
    }
    try {
      await start(original, "before");
      await waitHeight(3);
      assert.equal((await node.status()).nodeInfo.network, chain);
      const missing = await node.abciQuery({
        path: "/akash.sink.v1.Query/State",
        data: new Uint8Array(),
      });
      assert.notEqual(
        missing.code,
        0,
        "original chain unexpectedly exposes sink",
      );
      const bystander = addresses.get("bystander");
      assert.ok(bystander);
      const initial = {
        bystander: await bank(bystander),
        sink: await bank(sink),
      };
      // The dust send created a plain account at the sink address. The
      // upgrade must adopt it and move the dust out rather than halt.
      const dust =
        scenario === "preexisting-balance"
          ? { account: await account(sink), pool: await communityPool() }
          : undefined;
      if (dust) {
        assert.equal(initial.sink, "1");
        assert.equal(dust.account.module, "");
      }
      const upgradeHeight = (await height()) + 35;
      const proposal = join(home, "proposal.json");
      await save(proposal, {
        messages: [
          {
            "@type": "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade",
            authority: governanceAddress,
            plan: {
              name: "sink-local-v1",
              height: String(upgradeHeight),
              // Standard cosmovisor info; the handler does not read it.
              info: "",
            },
          },
        ],
        metadata: "",
        deposit: "100000000uakt",
        title: "Install sink on existing local chain",
        summary: "Add sink store with a zero balance",
      });
      await transact(["gov", "submit-proposal", proposal]);
      await transact(["gov", "vote", "1", "yes"]);
      const deadline = Date.now() + 75000;
      for (;;) {
        try {
          const info = object(
            JSON.parse(
              await readFile(join(home, "data/upgrade-info.json"), "utf8"),
            ),
          );
          if (info.name === "sink-local-v1") break;
        } catch {
          /* Not at scheduled halt yet. */
        }
        assert.ok(
          Date.now() < deadline,
          "old binary did not halt for the approved upgrade",
        );
        await sleep(500);
      }
      await stop();
      await start(AKASH, "upgrade");
      await waitHeight(upgradeHeight + 3);
      const reply = await node.abciQuery({
        path: "/akash.sink.v1.Query/State",
        data: new Uint8Array(),
      });
      assert.equal(reply.code, 0);
      const sinkState = QueryStateResponse.decode(reply.value);
      assert.equal(sinkState.params?.enabled, false);
      assert.equal(sinkState.totals?.lastTransferId.toBigInt(), 0n);
      assert.equal(await bank(sink), "0");
      assert.equal(await bank(bystander), initial.bystander);
      const adopted = await account(sink);
      assert.equal(adopted.module, "sink");
      if (dust) {
        assert.equal(adopted.number, dust.account.number);
        assert.ok((await communityPool()) >= dust.pool + 1n);
      }
      const afterHeight = await height();
      await stop();
      await start(AKASH, "restart");
      await waitHeight(afterHeight + 2);
      assert.equal(await bank(sink), "0");
      results.push({
        scenario,
        passed: true,
        chain,
        upgradeHeight,
        height: await height(),
        sinkBalance: "0",
        enabled: false,
        survivesRestart: true,
        bystanderUnchanged: true,
        ...(dust && {
          preexistingDustToCommunityPool: "1",
          accountNumberKept: dust.account.number?.toString(),
        }),
      });
      await save(join(root, "results.json"), results);
      console.log(JSON.stringify(results.at(-1)));
    } finally {
      await stop();
      node.disconnect();
    }
  }
  console.log(`Upgrade evidence: ${root}/results.json`);
}
void main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
