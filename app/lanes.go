package app

import (
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"

	signerextraction "github.com/skip-mev/block-sdk/adapters/signer_extraction_adapter"
	"github.com/skip-mev/block-sdk/block/base"
	defaultlane "github.com/skip-mev/block-sdk/lanes/base"
	mevlane "github.com/skip-mev/block-sdk/lanes/mev"
)

const (
	maxTxPerTopOfBlockAuctionLane = 500  // this is the maximum # of bids that will be held in the app-side in-memory mempool
	maxTxPerDefaultLane           = 3000 // all other txs
)

var (
	defaultLaneBlockspacePercentage           = math.LegacyMustNewDecFromStr("0.90")
	topOfBlockAuctionLaneBlockspacePercentage = math.LegacyMustNewDecFromStr("0.10")
)

// CreateLanes walks through the process of creating the lanes for the block sdk. In this function
// we create three separate lanes - MEV, Free, and Default - and then return them.
func CreateLanes(app *AkashApp, txConfig client.TxConfig) (*mevlane.MEVLane, *base.BaseLane) {
	// Create the signer extractor. This is used to extract the expected signers from
	// a transaction. Each lane can have a different signer extractor if needed.
	signerAdapter := signerextraction.NewDefaultAdapter()

	// Create the configurations for each lane. These configurations determine how many
	// transactions the lane can store, the maximum block space the lane can consume, and
	// the signer extractor used to extract the expected signers from a transaction.

	// Create a mev configuration that accepts maxTxPerTopOfBlockAuctionLane transactions and consumes topOfBlockAuctionLaneBlockspacePercentage of the
	// block space.
	mevConfig := base.LaneConfig{
		Logger:          app.Logger(),
		TxEncoder:       txConfig.TxEncoder(),
		TxDecoder:       txConfig.TxDecoder(),
		MaxBlockSpace:   topOfBlockAuctionLaneBlockspacePercentage,
		SignerExtractor: signerAdapter,
		MaxTxs:          maxTxPerTopOfBlockAuctionLane,
	}

	// Create a default configuration that accepts maxTxPerDefaultLane transactions and consumes defaultLaneBlockspacePercentage of the
	// block space.
	defaultConfig := base.LaneConfig{
		Logger:          app.Logger(),
		TxEncoder:       txConfig.TxEncoder(),
		TxDecoder:       txConfig.TxDecoder(),
		MaxBlockSpace:   defaultLaneBlockspacePercentage,
		SignerExtractor: signerAdapter,
		MaxTxs:          maxTxPerDefaultLane,
	}

	// Create the match handlers for each lane. These match handlers determine whether or not
	// a transaction belongs in the lane.

	// Create the final match handler for the mev lane.
	factory := mevlane.NewDefaultAuctionFactory(txConfig.TxDecoder(), signerAdapter)
	mevMatchHandler := factory.MatchHandler()

	// Create the final match handler for the default lane. I.e this will direct all txs that are
	// not free nor mev to this lane
	defaultMatchHandler := base.DefaultMatchHandler()

	// Create the lanes.
	mevLane := mevlane.NewMEVLane(
		mevConfig,
		factory,
		mevMatchHandler,
	)

	defaultLane := defaultlane.NewDefaultLane(
		defaultConfig,
		defaultMatchHandler,
	)

	return mevLane, defaultLane
}
