import { Registry, type EncodeObject } from "@cosmjs/proto-signing";
import {
  AminoTypes,
  createDefaultAminoConverters,
  defaultRegistryTypes,
} from "@cosmjs/stargate";
import {
  MsgGuardianPause,
  MsgSink,
  MsgSinkResponse,
} from "@akashnetwork/chain-sdk/private-types/akash.v1";

export const SINK_MSG_TYPE = "/akash.sink.v1.MsgSink";
export const GUARDIAN_PAUSE_TYPE = "/akash.sink.v1.MsgGuardianPause";
export const SINK_RESPONSE_TYPE = "/akash.sink.v1.MsgSinkResponse";
const MAX_UINT64 = 0xffffffffffffffffn;

export function parseSinkId(value: unknown): string {
  if (
    typeof value !== "string" ||
    !/^[1-9][0-9]{0,19}$/.test(value) ||
    BigInt(value) > MAX_UINT64
  )
    throw new Error(
      "Record ID must be a positive uint64 decimal without leading zeros",
    );
  return value;
}

const TRANSFER_KEY_PREFIX = [0x02, 0x00];
const VOID_KEY_PREFIX = [0x02, 0x01];

// An x/sink store key: a two-byte prefix, then the record ID as a big-endian
// u64. Uint8Array rather than Buffer because the browser UI imports this
// module.
function sinkKey(prefix: readonly number[], id: bigint | number) {
  const value = BigInt(id);
  if (value < 0n || value > MAX_UINT64)
    throw new RangeError("Record ID must be a uint64");
  const key = new Uint8Array(prefix.length + 8);
  key.set(prefix);
  new DataView(key.buffer).setBigUint64(prefix.length, value);
  return key;
}

// The 10-byte sink store key of transfer record `id`.
export function transferKey(id: bigint | number): Uint8Array {
  return sinkKey(TRANSFER_KEY_PREFIX, id);
}

// The 10-byte sink store key of the governance void order for `id`.
export function voidKey(id: bigint | number): Uint8Array {
  return sinkKey(VOID_KEY_PREFIX, id);
}

export function sinkMessage(input: {
  sender: string;
  dest: string;
  amountUakt: string;
}): EncodeObject & { value: MsgSink } {
  return {
    typeUrl: SINK_MSG_TYPE,
    value: {
      destType: 1,
      dest: input.dest,
      amount: { denom: "uakt", amount: input.amountUakt },
      sender: input.sender,
    },
  };
}

// The governance-set pause guardian can only turn deposits and/or refunds off.
export function guardianPauseMessage(input: {
  guardian: string;
  deposits: boolean;
  refunds: boolean;
}): EncodeObject & { value: MsgGuardianPause } {
  if (!input.deposits && !input.refunds)
    throw new Error("Pause deposits, refunds or both");
  return { typeUrl: GUARDIAN_PAUSE_TYPE, value: { ...input } };
}

export function sinkRegistry(): Registry {
  const registry = new Registry(defaultRegistryTypes);
  registry.register(SINK_MSG_TYPE, MsgSink);
  registry.register(GUARDIAN_PAUSE_TYPE, MsgGuardianPause);
  return registry;
}

function object(value: unknown): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    throw new Error("Invalid MsgSink Amino object");
  return Object.fromEntries(Object.entries(value));
}

function aminoMessage(value: unknown): MsgSink {
  const message = object(value);
  if (
    Object.keys(message).sort().join(",") !== "amount,dest,dest_type,sender" ||
    message.dest_type !== 1 ||
    typeof message.dest !== "string" ||
    typeof message.sender !== "string"
  )
    throw new Error("Invalid MsgSink Amino fields");
  const amount = object(message.amount);
  if (
    Object.keys(amount).sort().join(",") !== "amount,denom" ||
    amount.denom !== "uakt" ||
    typeof amount.amount !== "string" ||
    !/^[1-9][0-9]{0,19}$/.test(amount.amount) ||
    BigInt(amount.amount) > MAX_UINT64
  )
    throw new Error("Invalid MsgSink Amino amount");
  return {
    destType: 1,
    dest: message.dest,
    sender: message.sender,
    amount: { denom: "uakt", amount: amount.amount },
  };
}

// Amino JSON omits false booleans, matching the node's legacy encoding.
function aminoGuardianPause(value: unknown): MsgGuardianPause {
  const message = object(value);
  const keys = Object.keys(message);
  if (
    typeof message.guardian !== "string" ||
    !keys.every((key) => ["guardian", "deposits", "refunds"].includes(key)) ||
    (message.deposits !== undefined && message.deposits !== true) ||
    (message.refunds !== undefined && message.refunds !== true) ||
    (message.deposits !== true && message.refunds !== true)
  )
    throw new Error("Invalid MsgGuardianPause Amino fields");
  return {
    guardian: message.guardian,
    deposits: message.deposits === true,
    refunds: message.refunds === true,
  };
}

export function sinkAminoTypes(): AminoTypes {
  return new AminoTypes({
    ...createDefaultAminoConverters(),
    [SINK_MSG_TYPE]: {
      aminoType: "sink/MsgSink",
      toAmino(value: MsgSink) {
        const converted = {
          dest_type: value.destType,
          dest: value.dest,
          amount: value.amount,
          sender: value.sender,
        };
        aminoMessage(converted);
        return converted;
      },
      fromAmino: aminoMessage,
    },
    [GUARDIAN_PAUSE_TYPE]: {
      aminoType: "sink/MsgGuardianPause",
      toAmino(value: MsgGuardianPause) {
        const converted = {
          guardian: value.guardian,
          ...(value.deposits && { deposits: true }),
          ...(value.refunds && { refunds: true }),
        };
        aminoGuardianPause(converted);
        return converted;
      },
      fromAmino: aminoGuardianPause,
    },
  });
}

// A sink deposit must be the sole transaction message. Never infer its ID from
// the global counter or substitute an event for a missing typed response.
export function sinkResponseId(
  responses: readonly { typeUrl: string; value: Uint8Array }[],
): string {
  if (responses.length !== 1 || responses[0].typeUrl !== SINK_RESPONSE_TYPE)
    throw new Error("Expected exactly one MsgSinkResponse");
  return parseSinkId(MsgSinkResponse.decode(responses[0].value).id);
}
