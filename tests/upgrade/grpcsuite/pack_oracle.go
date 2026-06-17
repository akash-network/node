package grpcsuite

import (
	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	oraclev2 "pkg.akt.dev/go/node/oracle/v2"
	"pkg.akt.dev/go/sdkutil"
)

type oraclePack struct{}

func (oraclePack) Name() string { return "oracle" }

func (oraclePack) Available(d *Discovery) bool { return d.HasModule("akash.oracle.v2") }

func (oraclePack) Run(s *Suite) {
	q := oraclev2.NewQueryClient(s.Conn)
	writer := s.FundAccountDefault("pricewriter")

	// Authorize the writer as a price source (gov-gated param change).
	pr, err := q.Params(s.Ctx, &oraclev2.QueryParamsRequest{})
	require.NoError(s.T, err, "oracle Params")
	params := pr.Params
	params.Sources = append(params.Sources, writer.String())
	s.PassGovProposal("grpcsuite: authorize oracle price source",
		&oraclev2.MsgUpdateParams{Authority: s.GovAuthority(), Params: params})

	// Submit a price entry as the now-authorized source.
	s.feedAKTPrice(3)

	_, err = q.Prices(s.Ctx, &oraclev2.QueryPricesRequest{})
	require.NoError(s.T, err, "Prices")
	s.logf("oracle price entry submitted")
}

// feedAKTPrice submits a fresh AKT/USD oracle price from the authorized writer
// ("pricewriter", created by the oracle pack). The oracle requires the AKT/USD pair
// and a timestamp within ~12s of block time, so it uses the chain's latest block
// time. Other packs (bme) re-feed just before use so the price stays healthy.
func (s *Suite) feedAKTPrice(price int64) {
	s.T.Helper()
	s.BroadcastOK("pricewriter", &oraclev2.MsgAddPriceEntry{
		Signer:    s.Addr("pricewriter").String(),
		ID:        oraclev2.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD},
		Price:     sdkmath.LegacyNewDec(price),
		Timestamp: s.LatestBlockTime(),
	})
}
