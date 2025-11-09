package upgrade

import (
	"encoding/json"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// These files defines sdk specific types necessary to perform upgrade simulation.
// we're not using SDK generated types to prevent import of different types of cosmos sdk

type nodeStatus struct {
	SyncInfo struct {
		LatestBlockHeight string `json:"latest_block_height"`
		CatchingUp        bool   `json:"catching_up"`
	} `json:"sync_info"`
}

type votingParams struct {
	VotingPeriod string `json:"voting_period"`
}

type depositParams struct {
	MinDeposit sdk.Coins `json:"min_deposit"`
}

type govParams struct {
	VotingParams  votingParams  `json:"voting_params"`
	DepositParams depositParams `json:"deposit_params"`
}

type proposalResp struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type proposalsResp struct {
	Proposals []proposalResp `json:"proposals"`
}

type SoftwareUpgradeProposal struct {
	Type      string            `json:"@type"`
	Authority string            `json:"authority"`
	Plan      upgradetypes.Plan `json:"plan"`
}

type ProposalMsg struct {
	// Msgs defines an array of sdk.Msgs proto-JSON-encoded as Anys.
	Messages  []json.RawMessage `json:"messages,omitempty"`
	Metadata  string            `json:"metadata"`
	Deposit   string            `json:"deposit"`
	Title     string            `json:"title"`
	Summary   string            `json:"summary"`
	Expedited bool              `json:"expedited"`
}
