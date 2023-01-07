package migrate

import (
	"github.com/akash-network/node/x/market/types/v1beta1"
	"github.com/akash-network/node/x/market/types/v1beta2"
)

func LeaseIDToV1beta1(from v1beta1.LeaseID) v1beta2.LeaseID {
	return v1beta2.LeaseID{
		Owner:    from.Owner,
		DSeq:     from.DSeq,
		GSeq:     from.GSeq,
		OSeq:     from.OSeq,
		Provider: from.Provider,
	}
}
