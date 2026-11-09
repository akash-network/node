import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { parseArgs } from "node:util";
import { web3 } from "@coral-xyz/anchor";
import * as squads from "@sqds/multisig";
import { z } from "zod";
import { MAINNET_GENESIS } from "../shared/networks";
import { akt, decodeEvents, readEmitFile } from "../local/admin-instructions";

const confirm = {
  commitment: "finalized",
  preflightCommitment: "finalized",
} as const;
const publicKey = z.string().transform((value) => new web3.PublicKey(value));
const keyBytes = z.array(z.number().int().min(0).max(255)).length(64);
const json = async (path: string): Promise<unknown> =>
  JSON.parse(await readFile(path, "utf8"));
const keypair = async (path: string) =>
  web3.Keypair.fromSecretKey(Uint8Array.from(keyBytes.parse(await json(path))));
const save = (path: string, value: unknown, mode = 0o644) =>
  writeFile(path, JSON.stringify(value, null, 2) + "\n", { flag: "wx", mode });

async function main() {
  const {
    positionals: [command],
    values,
  } = parseArgs({
    allowPositionals: true,
    options: {
      url: { type: "string" },
      run: { type: "string" },
      bundle: { type: "string" },
    },
  });
  assert.ok(values.run, "Pass --run with the private rehearsal directory");
  const run = values.run;
  if (command === "fixture") {
    const connection = new web3.Connection(
      "https://api.mainnet-beta.solana.com",
      "finalized",
    );
    assert.equal(await connection.getGenesisHash(), MAINNET_GENESIS);
    const config = squads.getProgramConfigPda({})[0];
    const account = await connection.getAccountInfo(config, "finalized");
    assert.ok(
      account && account.owner.equals(squads.PROGRAM_ID),
      "invalid public Squads program config",
    );
    await save(join(run, "squads-config.json"), {
      pubkey: config.toBase58(),
      account: {
        ...account,
        owner: account.owner.toBase58(),
        data: [account.data.toString("base64"), "base64"],
        // RPC represents the rent-exempt u64 sentinel as an unsafe JS number.
        // The local fixture is rent exempt; it needs no mainnet rent epoch.
        rentEpoch: 0,
      },
    });
    process.stdout.write(config.toBase58() + "\n");
    return;
  }

  assert.ok(values.url, "Pass --url for an isolated loopback validator");
  assert.ok(
    ["127.0.0.1", "localhost", "[::1]"].includes(new URL(values.url).hostname),
  );
  const connection = new web3.Connection(values.url, "finalized");
  const genesis = await connection.getGenesisHash();
  assert.notEqual(
    genesis,
    MAINNET_GENESIS,
    "this helper never writes to mainnet",
  );
  const payer = await keypair(join(run, "keys/ceremony.json"));
  const send = (
    instructions: web3.TransactionInstruction[],
    signers: web3.Keypair[] = [payer],
  ) =>
    web3.sendAndConfirmTransaction(
      connection,
      new web3.Transaction().add(...instructions),
      signers,
      confirm,
    );

  if (command === "create") {
    const createKey = web3.Keypair.generate();
    const members = Array.from({ length: 5 }, () => web3.Keypair.generate());
    for (const [index, member] of members.entries())
      await save(
        join(run, `keys/member-${index}.json`),
        [...member.secretKey],
        0o600,
      );
    const multisigPda = squads.getMultisigPda({
      createKey: createKey.publicKey,
    })[0];
    const vault = squads.getVaultPda({ multisigPda, index: 0 })[0];
    const config = await squads.accounts.ProgramConfig.fromAccountAddress(
      connection,
      squads.getProgramConfigPda({})[0],
    );
    const signature = await send(
      [
        squads.instructions.multisigCreateV2({
          treasury: config.treasury,
          creator: payer.publicKey,
          multisigPda,
          configAuthority: null,
          threshold: 2,
          members: members.map((member) => ({
            key: member.publicKey,
            permissions: squads.types.Permissions.all(),
          })),
          timeLock: 5,
          createKey: createKey.publicKey,
          rentCollector: payer.publicKey,
        }),
        web3.SystemProgram.transfer({
          fromPubkey: payer.publicKey,
          toPubkey: vault,
          lamports: 10 * web3.LAMPORTS_PER_SOL,
        }),
      ],
      [payer, createKey],
    );
    await save(join(run, "squads.json"), {
      genesis,
      multisig: multisigPda.toBase58(),
      vault: vault.toBase58(),
      threshold: 2,
      timeLock: 5,
      signature,
    });
    process.stdout.write(vault.toBase58() + "\n");
    return;
  }

  if (command === "retired") {
    assert.ok(values.bundle, "Pass the old ceremony instruction bundle");
    const { file, authority, instructions } = readEmitFile(
      await json(values.bundle),
    );
    assert.equal(file.genesisHash, genesis);
    assert.ok(authority.equals(payer.publicKey));
    await assert.rejects(
      () => send(instructions),
      /InvalidAuthority|0x1b5a/,
      "the program must reject the retired ceremony authority",
    );
    process.stdout.write("Retired authority rejected by the Solana program.\n");
    return;
  }

  assert.equal(
    command,
    "execute",
    "expected fixture, create, execute, or retired",
  );
  assert.ok(
    values.bundle,
    "Pass --bundle with the reviewed vault instructions",
  );
  const state = z
    .object({ genesis: z.string(), multisig: publicKey, vault: publicKey })
    .parse(await json(join(run, "squads.json")));
  assert.equal(state.genesis, genesis);
  const { file, authority, instructions } = readEmitFile(
    await json(values.bundle),
  );
  assert.equal(file.genesisHash, genesis);
  assert.ok(
    authority.equals(state.vault),
    "bundle does not name the rehearsal vault",
  );
  const first = await keypair(join(run, "keys/member-0.json"));
  const second = await keypair(join(run, "keys/member-1.json"));
  const multisig = await squads.accounts.Multisig.fromAccountAddress(
    connection,
    state.multisig,
  );
  const transactionIndex = BigInt(multisig.transactionIndex.toString()) + 1n;
  const message = new web3.TransactionMessage({
    payerKey: state.vault,
    recentBlockhash: (await connection.getLatestBlockhash()).blockhash,
    instructions,
  });
  await send(
    [
      squads.instructions.vaultTransactionCreate({
        multisigPda: state.multisig,
        transactionIndex,
        creator: first.publicKey,
        rentPayer: payer.publicKey,
        vaultIndex: 0,
        ephemeralSigners: 0,
        transactionMessage: message,
      }),
      squads.instructions.proposalCreate({
        multisigPda: state.multisig,
        transactionIndex,
        creator: first.publicKey,
        rentPayer: payer.publicKey,
      }),
    ],
    [payer, first],
  );
  await send(
    [
      squads.instructions.proposalApprove({
        multisigPda: state.multisig,
        transactionIndex,
        member: first.publicKey,
      }),
    ],
    [payer, first],
  );
  const execute = async () => {
    const { instruction } = await squads.instructions.vaultTransactionExecute({
      connection,
      multisigPda: state.multisig,
      transactionIndex,
      member: first.publicKey,
    });
    return send([instruction], [payer, first]);
  };
  await assert.rejects(
    execute,
    /InvalidProposalStatus|0x1778/,
    "one approval must not execute a 2-of-5 proposal",
  );
  await send(
    [
      squads.instructions.proposalApprove({
        multisigPda: state.multisig,
        transactionIndex,
        member: second.publicKey,
      }),
    ],
    [payer, second],
  );
  const proposalAddress = squads.getProposalPda({
    multisigPda: state.multisig,
    transactionIndex,
  })[0];
  const proposal = await squads.accounts.Proposal.fromAccountAddress(
    connection,
    proposalAddress,
  );
  assert.ok(proposal.status.__kind === "Approved");
  const deadline =
    Number(proposal.status.timestamp.toString()) + multisig.timeLock;
  // Finalized confirmation can itself outlast this short rehearsal time lock.
  const now = await connection.getBlockTime(
    await connection.getSlot("finalized"),
  );
  assert.ok(now !== null);
  let checkedEarly = false;
  if (now < deadline) {
    await assert.rejects(
      execute,
      /TimeLockNotReleased|0x1785/,
      "approved transaction must wait for its time lock",
    );
    checkedEarly = true;
  }
  const waitUntil = Date.now() + 30_000;
  while (Date.now() < waitUntil) {
    const time = await connection.getBlockTime(
      await connection.getSlot("finalized"),
    );
    if (time !== null && time >= deadline) break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  const signature = await execute();
  const transaction = await connection.getTransaction(signature, {
    commitment: "finalized",
    maxSupportedTransactionVersion: 0,
  });
  assert.ok(transaction?.meta && transaction.meta.err === null);
  const aktProgram = file.summary.find(
    (entry) => entry.program === "akt",
  )?.programId;
  process.stdout.write(
    JSON.stringify(
      {
        signature,
        thresholdChecked: true,
        timeLockEarlyRejectionChecked: checkedEarly,
        events: aktProgram
          ? decodeEvents(
              transaction.meta.logMessages ?? [],
              akt(new web3.PublicKey(aktProgram)),
            )
          : [],
      },
      null,
      2,
    ) + "\n",
  );
}

void main().catch((error: unknown) => {
  process.stderr.write(
    `${error instanceof Error ? error.message : String(error)}\n`,
  );
  process.exitCode = 1;
});
