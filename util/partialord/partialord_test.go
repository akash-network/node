package partialord_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/util/partialord"
)

func TestAPI(t *testing.T) {
	// begin block use case, we have a dozen modules, but only care about a couple orders.
	// In practice this will be gotten from some API, e.g. app.AllModuleNames()
	moduleNames := []string{
		"auth", "authz", "bank", "capabilities",
		"staking", "distribution", "epochs", "mint", "upgrades", "wasm", "ibc",
		"ibctransfers",
	}
	beginBlockOrd := partialord.NewPartialOrdering(moduleNames)
	beginBlockOrd.FirstElements("upgrades", "epochs", "capabilities")
	beginBlockOrd.After("ibctransfers", "ibc")
	beginBlockOrd.Before("mint", "distribution")
	// This is purely just to test last functionality, doesn't make sense in context
	beginBlockOrd.LastElements("auth", "authz", "wasm")

	totalOrd := beginBlockOrd.TotalOrdering()
	expTotalOrd := []string{
		"upgrades", "epochs", "capabilities",
		"bank", "ibc", "mint", "staking", "ibctransfers", "distribution",
		"auth", "authz", "wasm",
	}
	require.Equal(t, expTotalOrd, totalOrd)
}

func TestNonStandardAPIOrder(t *testing.T) {
	// This test uses direct ordering before First, and after Last
	names := []string{"A", "B", "C", "D", "E", "F", "G"}
	ord := partialord.NewPartialOrdering(names)
	ord.After("A", "C")
	ord.After("A", "D")
	ord.After("E", "B")
	// overrides the "A" after "C" & "A" after "D" constraints
	ord.FirstElements("A", "B", "C")
	expOrdering := []string{"A", "B", "C", "D", "E", "F", "G"}
	require.Equal(t, expOrdering, ord.TotalOrdering())

	ord.After("E", "D")
	expOrdering = []string{"A", "B", "C", "D", "F", "G", "E"}
	require.Equal(t, expOrdering, ord.TotalOrdering())

	ord.LastElements("G")
	ord.After("F", "E")
	expOrdering = []string{"A", "B", "C", "D", "E", "F", "G"}
	require.Equal(t, expOrdering, ord.TotalOrdering())
}

// This test ad-hocly tests combination of multiple sequences, first elements, and an After
// invocation.
func TestSequence(t *testing.T) {
	// This test uses direct ordering before First, and after Last
	names := []string{"A", "B", "C", "D", "E", "F", "G"}
	ord := partialord.NewPartialOrdering(names)
	// Make B A C a sequence
	ord.Sequence("B", "A", "C")
	// Make A G E a sub-sequence
	ord.Sequence("A", "G", "E")
	// make first elements D B F
	ord.FirstElements("D", "B", "F")
	// make C come after E
	ord.After("C", "G")

	expOrdering := []string{"D", "B", "F", "A", "G", "C", "E"}
	require.Equal(t, expOrdering, ord.TotalOrdering())
}
