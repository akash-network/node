import { compareIds, sinkIdParts, ownerBytes, type SinkId } from "../shared/id";
import { coinAmount } from "../shared/proof";
import { challengeSeconds } from "../reporter/policy";
import assert from "node:assert/strict";
import { clientStatus } from "./client-status";
import { createHash } from "node:crypto";
import { join } from "node:path";
import { fromBech32, toBech32 } from "@cosmjs/encoding";
import { Program, web3 } from "@coral-xyz/anchor";
import { TxBody, TxRaw } from "cosmjs-types/cosmos/tx/v1beta1/tx";
import { TxMsgData } from "cosmjs-types/cosmos/base/abci/v1beta1/abci";
import { toRfc3339WithNanoseconds, type comet38 } from "@cosmjs/tendermint-rpc";
import clientIdl from "../idls/ics07_tendermint.json";
import type { Ics07Tendermint } from "../../../.cache/solana/deps/ibc-contracts/ibc-solana/target/types/ics07_tendermint";
import { AkashClient, SinkRecordNotFoundError } from "../reporter/akash";
import { parseConfig } from "../reporter/config";
import {
  DestType,
  Hold,
  Outcome,
  Sink_Status as Status,
  MsgSink,
  type TransferState,
  type Sink_Params,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";
import { parseSinkId, SINK_MSG_TYPE, sinkResponseId } from "../shared/sink";
import {
  SolanaObserver,
  SolanaUnavailableError,
  decodeRecord,
} from "../reporter/solana";
import type {
  AkashSettlement,
  BridgeConfig,
  BridgeReceipt,
  Deposit,
  DiscoveredSubmission,
  NetworkIdentity,
  ReceiptLookup,
  SolanaReceipt,
  TransferHistory,
} from "./team-contracts";
import {
  AKASH_RPC,
  CHAIN,
  LE64,
  PROGRAM_ID,
  ROOT,
  SOLANA_RPC,
  fee,
  json,
  object,
  pda,
} from "./common";

export class ApiFailure extends Error {
  constructor(
    readonly status: 400 | 404 | 422 | 503,
    message: string,
  ) {
    super(message);
  }
}

export function transactionHash(input: string | null) {
  if (!input || !/^[0-9a-fA-F]{64}$/.test(input))
    throw new ApiFailure(400, "A 64-character transaction hash is required");
  return input.toUpperCase();
}

export function receiptLookup(params: URLSearchParams) {
  const entries = [...params.entries()];
  if (entries.length !== 1)
    throw new ApiFailure(400, "Provide one transaction hash or record ID");
  const [key, value] = entries[0];
  if (key === "tx")
    return { kind: "transaction", hash: transactionHash(value) } as const;
  if (key === "id") {
    try {
      return { kind: "transfer", id: parseSinkId(value) } as const;
    } catch {
      throw new ApiFailure(400, "Invalid sink record ID");
    }
  }
  throw new ApiFailure(400, "Provide one transaction hash or record ID");
}

export function networkIdentity(
  input: Omit<NetworkIdentity, "networkKey">,
): NetworkIdentity {
  return {
    ...input,
    networkKey: JSON.stringify([
      input.chainId,
      input.akashGenesisTime,
      input.solanaGenesisHash,
      input.program,
      input.mint,
      input.sinkAddress,
    ]),
  };
}

async function context(akash: AkashClient) {
  const pinned = parseConfig(await json(join(ROOT, "reporter1.json")));
  assert.equal(pinned.akashRpc, AKASH_RPC);
  assert.equal(pinned.solanaRpc, SOLANA_RPC);
  assert.equal(pinned.akashChainId, CHAIN);
  assert.ok(pinned.programId.equals(PROGRAM_ID));
  assert.ok(pinned.mint.equals(pda("mint")));
  const initialized = object(await json(join(ROOT, "initialized.json")));
  assert.equal(initialized.chainId, CHAIN);
  assert.ok(typeof initialized.sinkAddress === "string");
  assert.equal(
    initialized.sinkAddress,
    toBech32(
      "akash",
      createHash("sha256").update("sink").digest().subarray(0, 20),
    ),
    "wrong native sink module address",
  );
  const sink = fromBech32(initialized.sinkAddress, 90);
  assert.equal(sink.prefix, "akash");
  assert.equal(sink.data.length, 20);
  assert.equal(toBech32("akash", sink.data), initialized.sinkAddress);
  const genesis = await akash.comet.genesis();
  assert.equal(genesis.chainId, CHAIN, "wrong Akash genesis");
  const network = networkIdentity({
    chainId: CHAIN,
    akashGenesisTime: toRfc3339WithNanoseconds(genesis.genesisTime),
    solanaGenesisHash: pinned.genesisHash,
    program: pinned.programId.toBase58(),
    mint: pinned.mint.toBase58(),
    sinkAddress: initialized.sinkAddress,
  });
  const connection = new web3.Connection(SOLANA_RPC, {
    commitment: "finalized",
    disableRetryOnRateLimit: true,
    fetch: (url, init) =>
      fetch(url, { ...init, signal: AbortSignal.timeout(12_000) }),
  });
  const observer = new SolanaObserver(connection, pinned);
  return { network, observer, connection };
}

export function lightClientStatus(input: {
  frozenHeight: bigint;
  latestHeight: bigint;
  consensusTimestampNs: bigint;
  trustingPeriodSeconds: bigint;
  solanaTimeSeconds: bigint;
}): BridgeConfig["client"] {
  return {
    status: clientStatus(input),
    latestHeight: input.latestHeight.toString(),
    ageSeconds: Number(
      input.solanaTimeSeconds - input.consensusTimestampNs / 1_000_000_000n,
    ),
    trustingPeriodSeconds: Number(input.trustingPeriodSeconds),
  };
}

async function lightClient(
  connection: web3.Connection,
  programId: web3.PublicKey,
) {
  const client = new Program<Ics07Tendermint>(
    { ...clientIdl, address: programId.toBase58() },
    { connection },
  );
  const state = await client.account.clientState.fetch(
    pda("client", programId),
  );
  assert.equal(state.chainId, CHAIN, "wrong light-client chain");
  const latestHeight = BigInt(state.latestHeight.revisionHeight.toString());
  const latest = await client.account.consensusStateStore.fetch(
    pda("consensus_state", programId, LE64(latestHeight)),
  );
  const time = await connection.getBlockTime(
    await connection.getSlot("confirmed"),
  );
  assert.ok(time !== null, "Solana clock unavailable");
  return lightClientStatus({
    frozenHeight: BigInt(state.frozenHeight.revisionHeight.toString()),
    latestHeight,
    consensusTimestampNs: BigInt(latest.consensusState.timestamp.toString()),
    trustingPeriodSeconds: BigInt(state.trustingPeriod.toString()),
    solanaTimeSeconds: BigInt(time),
  });
}

export function minTransferUakt(params: Sink_Params) {
  return coinAmount(params.minAmountPerTransfer, params.enabled).toString();
}
export function maxEpochAccountAmountUakt(params: Sink_Params) {
  return coinAmount(params.maxEpochAmountPerAccount).toString();
}

export function bridgeLimits(
  params: Sink_Params,
  bridgeEnabled: boolean,
): BridgeConfig["limits"] {
  return {
    enabled: params.enabled && bridgeEnabled,
    minTransferUakt: minTransferUakt(params),
    maxTransferUakt: coinAmount(params.maxAmountPerTransfer, true).toString(),
    maxEpochAmountTotalUakt: coinAmount(
      params.maxTransferAmountPerEpoch,
      true,
    ).toString(),
    maxEpochAmountPerAccountUakt: maxEpochAccountAmountUakt(params),
  };
}

export async function bridgeConfig(): Promise<BridgeConfig> {
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const { network, observer, connection } = await context(akash);
    const [state, deployment, client] = await Promise.all([
      akash.state(),
      observer.deployment(),
      observer
        .lightClient()
        .then((programId) => lightClient(connection, programId)),
    ]);
    return {
      version: 2,
      checkedAt: new Date().toISOString(),
      akashHeight: String(state.height),
      network,
      rpcPath: "/rpc/akash",
      restPath: "/rest/akash",
      denom: "uakt",
      decimals: 6,
      fee: { amountUakt: fee.amount[0].amount, gas: fee.gas },
      limits: bridgeLimits(
        state.params,
        deployment.bridgeEnabled && state.epoch > 0n,
      ),
      client,
    };
  } finally {
    akash.close();
  }
}

export function assertTransactionHash(
  tx: Pick<comet38.TxResponse, "tx" | "hash">,
  expected: string,
) {
  assert.equal(
    createHash("sha256").update(tx.tx).digest("hex").toUpperCase(),
    expected,
    "transaction bytes do not match requested hash",
  );
  assert.equal(
    Buffer.from(tx.hash).toString("hex").toUpperCase(),
    expected,
    "transaction response hash mismatch",
  );
}

export function depositFromTransaction(tx: comet38.TxResponse) {
  try {
    assert.equal(tx.result.code, 0);
    const body = TxBody.decode(TxRaw.decode(tx.tx).bodyBytes);
    assert.equal(body.messages.length, 1);
    const message = body.messages[0];
    assert.equal(message.typeUrl, SINK_MSG_TYPE);
    const deposit = MsgSink.decode(message.value);
    assert.equal(deposit.destType, DestType.DEST_TYPE_SOLANA);
    assert.equal(deposit.amount?.denom, "uakt");
    assert.ok(deposit.amount);
    const from = deposit.sender,
      amount = deposit.amount.amount,
      destination = deposit.dest;
    const id = sinkResponseId(
      TxMsgData.decode(tx.result.data ?? new Uint8Array()).msgResponses,
    );
    const identity = sinkIdParts(id);
    assert.equal(
      identity.owner,
      from,
      "response owner differs from signed sender",
    );
    assert.ok(Number.isSafeInteger(tx.height) && tx.height > 0);
    assert.equal(
      identity.height,
      BigInt(tx.height),
      "response height differs from transaction",
    );
    assert.match(amount, /^[1-9][0-9]{0,19}$/);
    assert.ok(BigInt(amount) <= 0xffffffffffffffffn);
    ownerBytes(from);
    const recipient = new web3.PublicKey(destination);
    assert.equal(recipient.toBase58(), destination);
    assert.ok(!recipient.equals(web3.PublicKey.default));
    return {
      id,
      sender: from,
      recipient: recipient.toBase58(),
      amountUakt: amount,
      height: String(tx.height),
    } satisfies Deposit;
  } catch {
    throw new ApiFailure(422, "Transaction is not a valid sink deposit");
  }
}

export function bindDepositRecord(deposit: Deposit, transfer: TransferState) {
  const record = decodeRecord(transfer);
  assert.equal(record.id.toString(), deposit.id, "deposit record ID mismatch");
  assert.equal(
    record.amount.toString(),
    deposit.amountUakt,
    "deposit record amount mismatch",
  );
  assert.equal(
    record.height.toString(),
    deposit.height,
    "deposit record height mismatch",
  );
  assert.equal(
    record.recipient.toBase58(),
    deposit.recipient,
    "deposit record recipient mismatch",
  );
  assert.equal(
    sinkIdParts(record.id).owner,
    deposit.sender,
    "deposit record sender mismatch",
  );
  return record;
}

function nativeStatus(status: Status): AkashSettlement["status"] {
  switch (status) {
    case Status.STATUS_TRANSFER_PENDING:
      return "transfer_pending";
    case Status.STATUS_VOIDED:
      return "void_requested";
    case Status.STATUS_MINTED:
      return "minted";
    case Status.STATUS_REFUND_PENDING:
      return "refund_pending";
    case Status.STATUS_REFUNDED:
      return "refunded";
    default:
      throw new Error("unsupported settlement status");
  }
}
function nativeHold(hold: Hold): AkashSettlement["hold"] {
  switch (hold) {
    case Hold.HOLD_NONE:
      return "none";
    case Hold.HOLD_VETO:
      return "veto";
    case Hold.HOLD_CONFLICT:
      return "conflict";
    default:
      throw new Error("unsupported settlement hold");
  }
}

export function akashSettlement(
  transfer: TransferState,
  state: {
    height: number;
    time: bigint;
    params: Pick<Sink_Params, "refundsEnabled">;
  },
): AkashSettlement {
  const settlement = transfer.settlement;
  assert.ok(settlement, "missing settlement");
  return {
    height: String(state.height),
    timeSeconds: state.time.toString(),
    status: nativeStatus(transfer.status),
    hold: nativeHold(settlement.hold),
    challengeEndSeconds:
      transfer.status === Status.STATUS_REFUND_PENDING
        ? challengeSeconds(settlement).toString()
        : null,
    refundsEnabled: state.params.refundsEnabled,
  };
}

export function depositFromRecord(transfer: TransferState): Deposit {
  const record = decodeRecord(transfer);
  return {
    id: record.id.toString(),
    sender: sinkIdParts(record.id).owner,
    recipient: record.recipient.toBase58(),
    amountUakt: record.amount.toString(),
    height: record.height.toString(),
  };
}

export async function bridgeReceipt(
  lookup: ReceiptLookup,
): Promise<BridgeReceipt> {
  const query =
    lookup.kind === "transaction"
      ? ({ kind: "transaction", hash: transactionHash(lookup.hash) } as const)
      : ({ kind: "transfer", id: parseSinkId(lookup.id) } as const);
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const { network, observer } = await context(akash);
    const state = await akash.state();
    const base = {
      version: 2 as const,
      networkKey: network.networkKey,
      checkedAt: new Date().toISOString(),
    };
    let expected: Deposit | undefined;
    let txHash: string | null = null;
    if (query.kind === "transaction") {
      txHash = query.hash;
      const search = await akash.comet.txSearch({
        query: `tx.hash='${txHash}'`,
        per_page: 2,
        page: 1,
      });
      assert.ok(search.totalCount <= 1 && search.txs.length <= 1);
      const tx = search.txs[0];
      if (!tx) return { ...base, txHash, kind: "unknown" };
      assertTransactionHash(tx, txHash);
      assert.ok(
        tx.height <= state.height,
        "transaction exceeds committed query height",
      );
      if (tx.result.code !== 0)
        return {
          ...base,
          txHash,
          kind: "rejected",
          height: String(tx.height),
          code: tx.result.code,
          message: "The Akash transaction failed; no deposit was accepted.",
        };
      expected = depositFromTransaction(tx);
    }
    const id = query.kind === "transfer" ? query.id : expected?.id;
    assert.ok(id, "missing sink record ID");
    let transfer: TransferState;
    try {
      transfer = await akash.transfer(parseSinkId(id), state.height);
    } catch (error) {
      if (error instanceof SinkRecordNotFoundError && query.kind === "transfer")
        throw new ApiFailure(
          404,
          "No sink record with this ID on this network",
        );
      throw error;
    }
    assert.ok(transfer?.settlement);
    const deposit = expected ?? depositFromRecord(transfer);
    const record = bindDepositRecord(deposit, transfer);
    assert.equal(deposit.id, id);
    assert.ok(
      record.height <= BigInt(state.height),
      "record exceeds query height",
    );
    let solana: SolanaReceipt;
    try {
      const observed = await observer.observeFinalized(record);
      solana =
        observed.kind === "absent"
          ? { kind: "absent", contextSlot: observed.contextSlot.toString() }
          : {
              kind:
                observed.outcome === Outcome.OUTCOME_MINTED
                  ? "minted"
                  : "voided",
              contextSlot: observed.contextSlot.toString(),
              resolvedSlot: observed.slot.toString(),
              proofHeight: observed.proofHeight.toString(),
            };
    } catch (error) {
      if (!(error instanceof SolanaUnavailableError)) throw error;
      solana = {
        kind: "unavailable",
        message: "Solana verification is temporarily unavailable.",
      };
    }
    return {
      ...base,
      txHash,
      kind: "deposit",
      deposit,
      akash: akashSettlement(transfer, state),
      solana,
    };
  } finally {
    akash.close();
  }
}

export function walletHistoryAddress(params: URLSearchParams) {
  try {
    const entries = [...params.entries()];
    assert.equal(entries.length, 1);
    const [key, address] = entries[0];
    assert.equal(key, "sender");
    ownerBytes(address);
    return address;
  } catch {
    throw new ApiFailure(400, "A canonical Akash sender address is required");
  }
}

const HISTORY_LIMIT = 100;
export async function ownerHistory(
  akash: Pick<AkashClient, "transfers" | "comet">,
  network: NetworkIdentity,
  sender: string,
  height: number,
): Promise<TransferHistory> {
  ownerBytes(sender);
  const page = await akash.transfers(height, {
    owner: sender,
    limit: HISTORY_LIMIT,
    reverse: true,
  });
  // Comet serves blocks from its base height. A record below the base was
  // imported at genesis or pruned with its block, so its time is unknown.
  const status = await akash.comet.status();
  assert.equal(status.nodeInfo.network, network.chainId, "wrong Akash chain");
  const base = status.syncInfo.earliestBlockHeight;
  assert.ok(
    base !== undefined &&
      Number.isSafeInteger(base) &&
      base > 0 &&
      base <= height,
    "Akash block history unavailable",
  );
  const times = new Map<string, string | null>();
  const transfers: DiscoveredSubmission[] = [];
  let previous: SinkId | undefined;
  for (const transfer of page.transfers) {
    const deposit = depositFromRecord(transfer),
      id = parseSinkId(deposit.id);
    assert.equal(deposit.sender, sender, "owner history mismatch");
    assert.ok(
      !previous || compareIds(id, previous) < 0,
      "owner history is not newest first",
    );
    previous = id;
    assert.ok(
      BigInt(deposit.height) <= BigInt(height),
      "history exceeds snapshot",
    );
    let createdAt = times.get(deposit.height);
    if (createdAt === undefined) {
      const blockHeight = Number(deposit.height);
      assert.ok(
        Number.isSafeInteger(blockHeight),
        "historical height cannot be represented by RPC client",
      );
      if (blockHeight < base) createdAt = null;
      else {
        const block = (await akash.comet.block(blockHeight)).block.header;
        assert.equal(block.chainId, network.chainId);
        assert.equal(block.height, blockHeight);
        createdAt = block.time.toISOString();
      }
      times.set(deposit.height, createdAt);
    }
    transfers.push({
      version: 2,
      networkKey: network.networkKey,
      txHash: null,
      transferId: deposit.id,
      sender,
      recipient: deposit.recipient,
      amountUakt: deposit.amountUakt,
      createdAt,
    });
  }
  return {
    version: 2,
    networkKey: network.networkKey,
    sender,
    transfers,
    truncated: page.nextKey.length > 0,
  };
}
export async function walletTransfers(sender: string) {
  const akash = new AkashClient(AKASH_RPC, CHAIN, 60);
  try {
    const { network } = await context(akash),
      state = await akash.state();
    return await ownerHistory(akash, network, sender, state.height);
  } finally {
    akash.close();
  }
}
