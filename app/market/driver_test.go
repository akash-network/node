package market_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/app/market/mocks"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"

	"github.com/tendermint/abci/types"
	ctypes "github.com/tendermint/tendermint/consensus/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

func TestDriver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	commitState, _ := testutil.NewState(t, nil)
	actor := market.NewActor(testutil.PrivateKey(t))

	bus := tmtmtypes.NewEventBus()
	require.NoError(t, bus.Start())
	defer func() { require.NoError(t, bus.Stop()) }()

	facilitator := new(mocks.Facilitator)
	facilitator.On("Run", commitState).Return(nil)

	driver, err := market.NewDriverWithFacilitator(ctx, testutil.Logger(), actor, bus, facilitator)
	require.NoError(t, err)

	// bogus events
	bus.PublishEventTx(tmtmtypes.EventDataTx{})
	bus.PublishEventNewBlock(tmtmtypes.EventDataNewBlock{})
	bus.PublishEventCompleteProposal(tmtmtypes.EventDataRoundState{})
	bus.PublishEventCompleteProposal(tmtmtypes.EventDataRoundState{RoundState: "something"})

	// make legit event
	validators := []*tmtmtypes.Validator{tmtmtypes.NewValidator(actor.PubKey(), 10)}
	vset := tmtmtypes.NewValidatorSet(validators)
	blk := tmtmtypes.MakeBlock(commitState.Version(), nil, &tmtmtypes.Commit{})
	blk.Header.ValidatorsHash = vset.Hash()
	bhash := blk.Hash()

	bus.PublishEventCompleteProposal(tmtmtypes.EventDataRoundState{
		Height: commitState.Version(),
		RoundState: &ctypes.RoundState{
			Validators:    vset,
			ProposalBlock: blk,
		},
	})
	testutil.SleepForThreadStart(t)

	// no commit
	require.NoError(t, driver.OnCommit(commitState))

	driver.OnBeginBlock(types.RequestBeginBlock{
		Hash: bhash,
	})

	// execute
	require.NoError(t, driver.OnCommit(commitState))

	driver.Stop()

	facilitator.AssertCalled(t, "Run", commitState)
}
