package grpcsuite

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

	// Params now lists the authorized writer.
	pr2, err := q.Params(s.Ctx, &oraclev2.QueryParamsRequest{})
	require.NoError(s.T, err, "oracle Params (after authorize)")
	require.Contains(s.T, pr2.Params.Sources, writer.String(), "writer should be an authorized source")

	// Prices: assert at least one entry exists after the feed.
	prices, err := q.Prices(s.Ctx, &oraclev2.QueryPricesRequest{})
	require.NoError(s.T, err, "Prices")
	require.NotEmpty(s.T, prices.Prices, "expected at least one price entry")

	// AggregatedPrice for the AKT denom.
	_, err = q.AggregatedPrice(s.Ctx, &oraclev2.QueryAggregatedPriceRequest{Denom: sdkutil.DenomAkt})
	require.NoError(s.T, err, "AggregatedPrice")

	oracleNegatives(s)
	s.logf("oracle price entry submitted")
}

// feedAKTPrice submits a fresh AKT/USD oracle price from the authorized writer
// ("pricewriter", created by the oracle pack). The oracle requires the AKT/USD pair
// and a timestamp within ~12s of block time, so it uses the chain's latest block
// time. Other packs (bme) re-feed just before use so the price stays healthy.
func (s *Suite) feedAKTPrice(price int64) {
	s.T.Helper()
	s.feedAKTPriceAs("pricewriter", s.Addr("pricewriter"), price)
}

func (s *Suite) feedAKTPriceAs(signer string, addr sdk.AccAddress, price int64) {
	s.T.Helper()
	s.BroadcastOK(signer, &oraclev2.MsgAddPriceEntry{
		Signer:    addr.String(),
		ID:        oraclev2.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD},
		Price:     sdkmath.LegacyNewDec(price),
		Timestamp: s.LatestBlockTime(),
	})
}

func oracleNegatives(s *Suite) {
	s.T.Helper()
	// Unauthorized account (not in Params.Sources) submitting a price.
	unauth := s.FundAccountDefault("pricewriter-unauth")
	s.BroadcastExpectErr("pricewriter-unauth", &oraclev2.MsgAddPriceEntry{
		Signer:    unauth.String(),
		ID:        oraclev2.DataID{Denom: sdkutil.DenomAkt, BaseDenom: sdkutil.DenomUSD},
		Price:     sdkmath.LegacyNewDec(1),
		Timestamp: s.LatestBlockTime(),
	})
	// Authorized writer, but an unsupported denom pair (only AKT/USD is accepted).
	s.BroadcastExpectErr("pricewriter", &oraclev2.MsgAddPriceEntry{
		Signer:    s.Addr("pricewriter").String(),
		ID:        oraclev2.DataID{Denom: "uatom", BaseDenom: sdkutil.DenomUSD},
		Price:     sdkmath.LegacyNewDec(1),
		Timestamp: s.LatestBlockTime(),
	})
}
