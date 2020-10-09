package testutil

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"math/rand"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/crypto/ed25519"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

func Keyring(t testing.TB) keyring.Keyring {
	obj := keyring.NewInMemory()
	return obj
}

// AccAddress provides an Account's Address bytes from a ed25519 generated
// private key.
func AccAddress(t testing.TB) sdk.AccAddress {
	t.Helper()
	privKey := ed25519.GenPrivKey()
	return sdk.AccAddress(privKey.PubKey().Address())
}

func DeploymentID(t testing.TB) dtypes.DeploymentID {
	t.Helper()
	return dtypes.DeploymentID{
		Owner: AccAddress(t).String(),
		DSeq:  uint64(rand.Uint32()),
	}
}

// DeploymentVersion provides a random sha256 sum for simulating Deployments.
func DeploymentVersion(t testing.TB) []byte {
	t.Helper()
	src := make([]byte, 128)
	_, err := cryptorand.Read(src)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(src)
	return sum[:]
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
