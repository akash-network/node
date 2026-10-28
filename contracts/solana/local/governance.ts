import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { toBech32 } from "@cosmjs/encoding";
import {
  MsgSubmitProposal,
  MsgSubmitProposalResponse,
  MsgVote,
} from "cosmjs-types/cosmos/gov/v1/tx";
import { VoteOption } from "cosmjs-types/cosmos/gov/v1/gov";
import { cosmosSigner, fee, sleep } from "./common";
export const governanceAddress = toBech32(
  "akash",
  createHash("sha256").update("gov").digest().subarray(0, 20),
);
export async function govern(
  title: string,
  messages: { typeUrl: string; value: Uint8Array }[],
  applied: () => Promise<boolean>,
) {
  const proposer = await cosmosSigner("validator0");
  let proposalId: bigint;
  try {
    const result = await proposer.client.signAndBroadcast(
      proposer.address,
      [
        {
          typeUrl: "/cosmos.gov.v1.MsgSubmitProposal",
          value: MsgSubmitProposal.fromPartial({
            proposer: proposer.address,
            title,
            summary: title,
            initialDeposit: [{ denom: "uakt", amount: "100000000" }],
            messages,
          }),
        },
      ],
      { amount: [{ denom: "uakt", amount: "50000" }], gas: "2000000" },
    );
    assert.equal(result.code, 0, result.rawLog);
    const response = result.msgResponses.find(
      (m) => m.typeUrl === "/cosmos.gov.v1.MsgSubmitProposalResponse",
    );
    assert.ok(response);
    proposalId = MsgSubmitProposalResponse.decode(response.value).proposalId;
  } finally {
    proposer.client.disconnect();
  }
  for (let i = 0; i < 4; i++) {
    const voter = await cosmosSigner(`validator${i}`);
    try {
      const tx = await voter.client.signAndBroadcast(
        voter.address,
        [
          {
            typeUrl: "/cosmos.gov.v1.MsgVote",
            value: MsgVote.fromPartial({
              proposalId,
              voter: voter.address,
              option: VoteOption.VOTE_OPTION_YES,
            }),
          },
        ],
        fee,
      );
      assert.equal(tx.code, 0, tx.rawLog);
    } finally {
      voter.client.disconnect();
    }
  }
  const deadline = Date.now() + 60_000;
  while (!(await applied())) {
    assert.ok(Date.now() < deadline, `proposal ${proposalId} did not apply`);
    await sleep(1000);
  }
  return proposalId;
}
