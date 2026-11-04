import { MerkleProof } from "cosmjs-types/ibc/core/commitment/v1/commitment";
import assert from "node:assert/strict";
import { z } from "zod";
import { parseSinkId, sinkIdParts, type SinkId } from "../shared/id";
import {
  MAX_PROOF_SIZE,
  PROOF_HEADER_SIZE,
  proofNonce,
  transferKey,
  settlementKey,
} from "../shared/proof";

const hash = z.string().regex(/^[a-f0-9]{64}$/);
const positive = z.number().int().positive().safe();
const bytes = z.string().regex(/^(?:[a-f0-9]{2})+$/);
const identity = z.object({
  chainId: z.string().min(1),
  genesis: z.string().min(1),
  programId: z.string().min(1),
  payer: z.string().min(1),
});
export type ProofIdentity = z.infer<typeof identity>;
const schema = identity
  .extend({
    version: z.literal(1),
    id: z.string().transform(parseSinkId),
    lightClient: z.string().min(1),
    proofHeight: positive,
    key: bytes,
    nonce: hash,
    valueHash: hash,
    proofHash: hash,
    proof: bytes,
    uploaded: z.number().int().min(0).max(MAX_PROOF_SIZE),
    allocated: z
      .number()
      .int()
      .min(0)
      .max(PROOF_HEADER_SIZE + MAX_PROOF_SIZE),
    lamports: z.number().int().nonnegative().safe(),
  })
  .strict();
export type ProofJob = z.infer<typeof schema>;

export function newProofJob(
  input: ProofIdentity & {
    id: SinkId;
    lightClient: string;
    proofHeight: number;
    key: Uint8Array;
    value: Uint8Array;
    proof: Uint8Array;
  },
): ProofJob {
  return parseProofJob(
    {
      chainId: input.chainId,
      genesis: input.genesis,
      programId: input.programId,
      payer: input.payer,
      id: input.id,
      lightClient: input.lightClient,
      proofHeight: input.proofHeight,
      version: 1,
      key: Buffer.from(input.key).toString("hex"),
      nonce: proofNonce(input.key).toString("hex"),
      valueHash: proofNonce(input.value).toString("hex"),
      proofHash: proofNonce(input.proof).toString("hex"),
      proof: Buffer.from(input.proof).toString("hex"),
      uploaded: 0,
      allocated: 0,
      lamports: 0,
    },
    input,
    input.id,
  );
}

export function parseProofJob(
  value: unknown,
  deployment: ProofIdentity,
  id: SinkId,
): ProofJob {
  const job = schema.parse(value);
  for (const key of ["chainId", "genesis", "programId", "payer"] as const)
    assert.equal(
      job[key],
      deployment[key],
      "proof job belongs to another deployment",
    );
  assert.equal(job.id, id, "proof job identity mismatch");
  const key = Buffer.from(job.key, "hex"),
    proof = Buffer.from(job.proof, "hex");
  assert.equal(
    job.nonce,
    proofNonce(key).toString("hex"),
    "proof job nonce mismatch",
  );
  assert.equal(
    job.proofHash,
    proofNonce(proof).toString("hex"),
    "proof job bytes changed",
  );
  assert.ok(
    proof.length > 0 && proof.length <= MAX_PROOF_SIZE,
    "proof exceeds upload bound",
  );
  assert.ok(
    BigInt(job.proofHeight) > sinkIdParts(id).height,
    "proof job predates transfer",
  );
  assert.ok(
    Buffer.from(transferKey(id)).equals(key) ||
      (key.length > 2 && Buffer.from(settlementKey(id, key[2])).equals(key)),
    "proof job key mismatch",
  );
  const decoded = MerkleProof.decode(proof);
  assert.equal(decoded.proofs.length, 2, "proof job needs two proof levels");
  const [iavl, store] = decoded.proofs;
  assert.ok(iavl.exist && store.exist, "proof job needs existence values");
  assert.deepEqual(
    Buffer.from(iavl.exist.key),
    key,
    "proof job existence key mismatch",
  );
  assert.deepEqual(
    Buffer.from(store.exist.key),
    Buffer.from("sink"),
    "proof job store mismatch",
  );
  assert.equal(
    proofNonce(iavl.exist.value).toString("hex"),
    job.valueHash,
    "proof job value hash mismatch",
  );
  assert.ok(job.uploaded <= proof.length, "invalid upload progress");
  return job;
}

export function sameProof(a: ProofJob, b: ProofJob) {
  return (
    a.key === b.key &&
    a.valueHash === b.valueHash &&
    a.proofHash === b.proofHash &&
    a.proofHeight === b.proofHeight &&
    a.lightClient === b.lightClient
  );
}
