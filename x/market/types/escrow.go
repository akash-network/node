package types

import (
	fmt "fmt"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	etypes "github.com/ovrclk/akash/x/escrow/types"
)

const (
	bidEscrowScope = "bid"
)

func EscrowAccountForBid(id BidID) etypes.AccountID {
	return etypes.AccountID{
		Scope: bidEscrowScope,
		XID:   id.String(),
	}
}

func EscrowPaymentForBid(id BidID) string {
	return fmt.Sprintf("%v/%v/%s", id.GSeq, id.OSeq, id.Provider)
}
func BidIDFromEscrowAccount(id etypes.AccountID, pid string) (BidID, bool) {
	did, ok := dtypes.DeploymentIDFromEscrowAccount(id)
	if !ok {
		return BidID{}, false
	}

	parts := strings.Split(pid, "/")
	if len(parts) != 3 {
		return BidID{}, false
	}

	gseq, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return BidID{}, false
	}

	oseq, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return BidID{}, false
	}

	owner, err := sdk.AccAddressFromBech32(parts[2])
	if err != nil {
		return BidID{}, false
	}

	return MakeBidID(
		MakeOrderID(
			dtypes.MakeGroupID(
				did, uint32(gseq)), uint32(oseq)), owner), true
}
