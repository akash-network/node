package v1beta1

import (
	"fmt"
	"strconv"
	"strings"

	dtypes "github.com/akash-network/node/x/deployment/types/v1beta1"
	etypes "github.com/akash-network/node/x/escrow/types/v1beta1"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

func EscrowPaymentForLease(id LeaseID) string {
	return fmt.Sprintf("%v/%v/%s", id.GSeq, id.OSeq, id.Provider)
}

func LeaseIDFromEscrowAccount(id etypes.AccountID, pid string) (LeaseID, bool) {
	did, ok := dtypes.DeploymentIDFromEscrowAccount(id)
	if !ok {
		return LeaseID{}, false
	}

	parts := strings.Split(pid, "/")
	if len(parts) != 3 {
		return LeaseID{}, false
	}

	gseq, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return LeaseID{}, false
	}

	oseq, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return LeaseID{}, false
	}

	owner, err := sdk.AccAddressFromBech32(parts[2])
	if err != nil {
		return LeaseID{}, false
	}

	return MakeLeaseID(
		MakeBidID(
			MakeOrderID(
				dtypes.MakeGroupID(
					did, uint32(gseq)), uint32(oseq)), owner)), true
}
