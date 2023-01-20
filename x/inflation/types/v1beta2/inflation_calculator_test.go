package v1beta2

// todo akash-network/support#4
// import (
// 	"testing"
// 	"time"
//
// 	sdk "github.com/cosmos/cosmos-sdk/types"
// 	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
// 	"github.com/stretchr/testify/suite"
// )
//
// const (
// 	blocksPerYear = 5256000
// )
//
// type InflationCalculatorTestSuite struct {
// 	suite.Suite
// 	params      Params
// 	genesistime time.Time
// }
//
// func (s *InflationCalculatorTestSuite) SetupSuite() {
// 	var err error
// 	s.genesistime, err = time.Parse(time.RFC3339, "2021-03-08T15:00:00Z")
// 	s.Require().NoError(err)
//
// 	s.params.InflationDecayFactor, err = sdk.NewDecFromStr("2.10306569")
// 	s.Require().NoError(err)
//
// 	s.params.InitialInflation, err = sdk.NewDecFromStr("48.546257")
// 	s.Require().NoError(err)
//
// 	s.params.Variance, err = sdk.NewDecFromStr("0.05")
// 	s.Require().NoError(err)
// }
//
// func TestIntegrationTestSuite(t *testing.T) {
// 	suite.Run(t, new(InflationCalculatorTestSuite))
// }
//
// func (s *InflationCalculatorTestSuite) TestInflationCalculatorInvalidDecayFactor() {
// 	testFn := func() {
// 		inflationCalculator(
// 			time.Time{},
// 			time.Time{},
// 			minttypes.Minter{},
// 			minttypes.Params{},
// 			Params{},
// 			sdk.Dec{})
// 	}
//
// 	s.Panics(testFn)
// }
//
// func (s *InflationCalculatorTestSuite) TestInflationCalculator1() {
// 	goalBonded, err := sdk.NewDecFromStr("0.67")
// 	s.Require().NoError(err)
//
// 	currBonded, err := sdk.NewDecFromStr("0.7324")
// 	s.Require().NoError(err)
//
// 	currInflation, err := sdk.NewDecFromStr("0.230326319830867266")
// 	s.Require().NoError(err)
//
// 	blockTime, _ := time.Parse(time.RFC3339, "2022-04-18T18:28:26+00:00")
//
// 	res := inflationCalculator(
// 		blockTime,
// 		s.genesistime,
// 		minttypes.Minter{
// 			Inflation: currInflation,
// 		},
// 		minttypes.Params{
// 			BlocksPerYear:       blocksPerYear,
// 			GoalBonded:          goalBonded,
// 			InflationRateChange: s.params.Variance,
// 		},
// 		s.params,
// 		currBonded)
//
// 	s.Require().Equal("31.967899564902300000", res.String())
// }
