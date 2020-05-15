package kube

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/manifest"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
)

/* Source: https://git.sr.ht/~samwhited/testlog/tree/b1b3e8e82fd6990e91ce9d0fbcbe69ac2d9b1f98/testlog.go
// New returns a new logger that logs to the provided testing.T.
func New(t testing.TB) *log.Logger {
	t.Helper()
	return log.New(testWriter{TB: t}, t.Name()+" ", log.LstdFlags|log.Lshortfile|log.LUTC)
}
*/

type testWriter struct {
	testing.TB
}

func (tw testWriter) Write(p []byte) (int, error) {
	tw.Helper()
	tw.Logf("%s", p)
	return len(p), nil
}

func testLeaseID() mtypes.LeaseID {
	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     randDSeq,
		GSeq:     randGSeq,
		OSeq:     randOSeq,
		Provider: sdk.AccAddress(provider),
	}
	return leaseID
}

func TestLidNsSanity(t *testing.T) {
	owner := ed25519.GenPrivKey().PubKey().Address()
	provider := ed25519.GenPrivKey().PubKey().Address()
	tw := testWriter{TB: t}
	log := log.NewTMLogger(tw)

	leaseID := mtypes.LeaseID{
		Owner:    sdk.AccAddress(owner),
		DSeq:     randDSeq,
		GSeq:     randGSeq,
		OSeq:     randOSeq,
		Provider: sdk.AccAddress(provider),
	}
	g := &manifest.Group{}

	shaNS := lidNS(leaseID)
	t.Logf("sha256: %q", shaNS)
	if shaNS == "" {
		t.Errorf("sha* should not be empty")
	}

	mb := newManifestBuilder(log, shaNS, leaseID, g)

	m, err := mb.create()
	if err != nil {
		t.Fatal(err)
	}

	if m.Name != shaNS {
		t.Errorf("k8s namespace: %q does not match %q", m.Namespace, shaNS)
	}
}

func TestShaLengths(t *testing.T) {
	lid := testLeaseID()
	path := mtypes.BidIDString(lid.BidID())

	sha256s := sha256.Sum256([]byte(path))
	hex256 := hex.EncodeToString(sha256s[:])
	t.Logf("sha256 sum: %d hex: %d", len(sha256s), len(hex256))

	/* Disabled to escape `gosec` lint error on usage of sha1
	sha1s := sha1.Sum([]byte(path))
	hex1 := hex.EncodeToString(sha1s[:])
	t.Logf("sha1 sum: %d hex: %d", len(sha1s), len(hex1))
	*/
}
