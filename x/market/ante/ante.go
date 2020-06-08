package ante

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type marketDecorator struct {
	mkeeper keeper.Keeper
	pkeeper provider.Keeper
}

// NewDecorator returns a decorator for "deployment" type messages
func NewDecorator(mkeeper keeper.Keeper, pkeeper provider.Keeper) sdk.AnteDecorator {
	return marketDecorator{
		mkeeper: mkeeper,
		pkeeper: pkeeper,
	}
}

func (dd marketDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		var err error
		switch msg := msg.(type) {
		case types.MsgCreateBid:
			err = dd.handleMsgCreateBid(ctx, msg)
		case types.MsgCloseBid:
			err = dd.handleMsgCloseBid(ctx, msg)
		case types.MsgCloseOrder:
			err = dd.handleMsgCloseOrder(ctx, msg)
		default:
			continue
		}

		if err != nil {
			return ctx, err
		}
	}
	return next(ctx, tx, simulate)
}

func (dd marketDecorator) handleMsgCreateBid(ctx sdk.Context, msg types.MsgCreateBid) error {
	order, ok := dd.mkeeper.GetOrder(ctx, msg.Order)
	if !ok {
		return types.ErrInvalidOrder
	}

	if err := order.ValidateCanBid(); err != nil {
		return types.ErrInternal
	}

	if order.Price().IsLT(msg.Price) {
		return types.ErrBidOverOrder
	}

	var prov ptypes.Provider
	if prov, ok = dd.pkeeper.Get(ctx, msg.Provider); !ok {
		return types.ErrEmptyProvider
	}

	if !order.MatchAttributes(prov.Attributes) {
		return types.ErrAtributeMismatch
	}

	return nil
}

func (dd marketDecorator) handleMsgCloseBid(ctx sdk.Context, msg types.MsgCloseBid) error {
	bid, ok := dd.mkeeper.GetBid(ctx, msg.BidID)
	if !ok {
		return types.ErrUnknownBid
	}

	lease, ok := dd.mkeeper.GetLease(ctx, types.LeaseID(msg.BidID))
	if !ok {
		return types.ErrUnknownLeaseForBid
	}

	if _, ok = dd.mkeeper.GetOrder(ctx, msg.OrderID()); !ok {
		return types.ErrUnknownOrderForBid
	}

	if lease.State != types.LeaseActive {
		return types.ErrLeaseNotActive
	}

	if bid.State != types.BidMatched {
		return types.ErrBidNotMatched
	}

	return nil
}

func (dd marketDecorator) handleMsgCloseOrder(ctx sdk.Context, msg types.MsgCloseOrder) error {
	order, ok := dd.mkeeper.GetOrder(ctx, msg.OrderID)
	if !ok {
		return types.ErrUnknownOrder
	}

	if _, ok = dd.mkeeper.LeaseForOrder(ctx, order.ID()); !ok {
		return types.ErrNoLeaseForOrder
	}

	return nil
}
