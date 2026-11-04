const assert = require("node:assert/strict");
const { randomBytes } = require("node:crypto");
const { mkdir, readFile, writeFile } = require("node:fs/promises");
const { join } = require("node:path");
const { DirectSecp256k1Wallet } = require("@cosmjs/proto-signing");
const { web3 } = require("@coral-xyz/anchor");

const state = process.env.SINK_STATE;
assert.ok(state, "SINK_STATE is required");
const save = (name, value) =>
  writeFile(join(state, name), JSON.stringify(value, null, 2) + "\n", {
    flag: "wx",
    mode: 0o600,
  });

async function main() {
  switch (process.argv[2]) {
    case "keys": {
      await mkdir(state, { recursive: true, mode: 0o700 });
      const reporters = [];
      for (let i = 0; i < 7; i++) {
        const key = randomBytes(32);
        const wallet = await DirectSecp256k1Wallet.fromKey(key, "akash");
        await writeFile(join(state, `reporter${i}.key`), key.toString("hex"), {
          flag: "wx",
          mode: 0o600,
        });
        reporters.push((await wallet.getAccounts())[0].address);
      }
      await save("reporters.json", reporters);
      break;
    }
    case "configs": {
      const response = await fetch("http://127.0.0.1:26657/status", {
        signal: AbortSignal.timeout(10_000),
      });
      assert.ok(response.ok);
      const chain = (await response.json()).result.node_info.network;
      assert.ok(typeof chain === "string" && chain.length > 0);
      assert.notEqual(chain, "akashnet-2", "use an isolated local fork");
      const solana = new web3.Connection("http://127.0.0.1:8899", "finalized");
      const genesis = await solana.getGenesisHash();
      const programId = process.argv[3];
      const mint = web3.PublicKey.findProgramAddressSync(
        [Buffer.from("mint")],
        new web3.PublicKey(programId),
      )[0].toBase58();
      const common = {
        akashRpc: "http://127.0.0.1:26657",
        akashChainId: chain,
        solanaRpc: "http://127.0.0.1:8899",
        solanaGenesisHash: genesis,
        programId,
      };
      await save("relayer.json", {
        ...common,
        payerKeyFile: join(state, "payer.json"),
        stateDir: join(state, "relayer"),
        headerEncoder:
          "/workspace/.cache/solana/target/debug/akt-header-encoder",
      });
      const reporters = JSON.parse(
        await readFile(join(state, "reporters.json"), "utf8"),
      );
      for (const [i, reporter] of reporters.entries()) {
        await save(`reporter${i}.json`, {
          ...common,
          reporter,
          mint,
          maxAgeSeconds: 60,
          pollSeconds: 3,
          feeAmountUakt: "12500",
          gasLimit: "500000",
        });
      }
      await save("network.json", {
        chainId: chain,
        solanaGenesisHash: genesis,
        programId,
        mint,
      });
      break;
    }
    case "invalid-recipient": {
      const { createAccount } = require("@solana/spl-token");
      const {
        loadPayer,
      } = require("/workspace/contracts/solana/local/relay-config.ts");
      const network = JSON.parse(
        await readFile(join(state, "network.json"), "utf8"),
      );
      const payer = await loadPayer(join(state, "payer.json"));
      const account = await createAccount(
        new web3.Connection("http://127.0.0.1:8899", "finalized"),
        payer,
        new web3.PublicKey(network.mint),
        payer.publicKey,
        web3.Keypair.generate(),
      );
      console.log(account.toBase58());
      break;
    }
    default:
      throw new Error(
        "usage: identities.cjs keys|configs PROGRAM_ID|invalid-recipient",
      );
  }
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
