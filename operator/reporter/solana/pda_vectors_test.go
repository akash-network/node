package solana_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// pdaVectors are addresses the TS reporter derived (contracts/solana/reporter/goldens.ts): statusAddress in
// contracts/solana/shared/proof.ts, and findProgramAddressSync for the deployment seeds.
type pdaVectors struct {
	Status []struct {
		ProgramID string `json:"programId"`
		ID        string `json:"id"`
		Address   string `json:"address"`
	} `json:"status"`
	Seeds []struct {
		ProgramID string `json:"programId"`
		Seed      string `json:"seed"`
		Address   string `json:"address"`
	} `json:"seeds"`
}

func TestPDAVectors(t *testing.T) {
	raw, err := os.ReadFile("../testdata/pda_vectors.json")
	require.NoError(t, err)

	var v pdaVectors
	require.NoError(t, json.Unmarshal(raw, &v))
	require.NotEmpty(t, v.Status)
	require.NotEmpty(t, v.Seeds)

	for _, c := range v.Status {
		id, err := sinkid.Parse(c.ID)
		require.NoError(t, err, c.ID)

		t.Run(fmt.Sprintf("status %s owner %d bytes height %d", c.ProgramID, len(id.OwnerBytes), id.Height), func(t *testing.T) {
			got := aktsolana.StatusAddress(solana.MustPublicKeyFromBase58(c.ProgramID), id)
			require.Equal(t, c.Address, got.String())
		})
	}

	seeds := map[string]bool{}
	for _, c := range v.Seeds {
		seeds[c.Seed] = true
		t.Run(c.Seed+" "+c.ProgramID, func(t *testing.T) {
			got := aktsolana.PDA(solana.MustPublicKeyFromBase58(c.ProgramID), []byte(c.Seed))
			require.Equal(t, c.Address, got.String())
		})
	}
	require.Equal(t, map[string]bool{"config": true, "bridge": true, "mint": true, "mint_authority": true}, seeds)
}
