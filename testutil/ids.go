package testutil

import (
	"math/rand"
	"testing"

	ckeys "github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/crypto/keys"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/stretchr/testify/require"
)

func Keyring(t testing.TB) keys.Keybase {
	obj := keys.NewInMemory()
	return obj
}

func AccAddress(t testing.TB) sdk.AccAddress {
	t.Helper()
	info, _, err := Keyring(t).CreateMnemonic("test", keys.English, ckeys.DefaultKeyPass, keys.Secp256k1)
	require.NoError(t, err)
	return info.GetAddress()
}

func DeploymentID(t testing.TB) dtypes.DeploymentID {
	t.Helper()
	return dtypes.DeploymentID{
		Owner: AccAddress(t),
		DSeq:  uint64(rand.Uint32()),
	}
}

func GroupID(t testing.TB) dtypes.GroupID {
	t.Helper()
	return dtypes.MakeGroupID(DeploymentID(t), rand.Uint32())
}

func OrderID(t testing.TB) mtypes.OrderID {
	t.Helper()
	return mtypes.MakeOrderID(GroupID(t), rand.Uint32())
}

func BidID(t testing.TB) mtypes.BidID {
	t.Helper()
	return mtypes.MakeBidID(OrderID(t), AccAddress(t))
}

func LeaseID(t testing.TB) mtypes.LeaseID {
	t.Helper()
	return mtypes.MakeLeaseID(BidID(t))
}
