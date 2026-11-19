package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/telemetry"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// BeginBlocker is called at the beginning of each block
func (k *keeper) BeginBlocker(ctx context.Context) error {
	startTm := telemetry.Now()
	defer telemetry.ModuleMeasureSince(types.ModuleName, startTm, telemetry.MetricKeyBeginBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)

	// The allocator stores the last issued sequence and nextID issues the
	// stored value, so a reset to one makes each block start at 1.
	err := k.sequence.Set(ctx, 1)
	if err != nil {
		sctx.Logger().Error("failed to reset ledger sequence", "err", err)
	}

	totals, err := k.totals.Get(sctx)
	if err != nil {
		return err
	}

	err = k.totalsCurr.Set(sctx, totals)
	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) EndBlocker(ctx context.Context) error {
	sctx := sdk.UnwrapSDKContext(ctx)

	t, err := k.totals.Get(sctx)
	if err != nil {
		return err
	}

	if err = t.Validate(); err != nil {
		return err
	}

	escrow, err := t.EscrowAmount()
	if err != nil {
		return err
	}

	expected := sdk.NewCoins(escrow)
	if !k.bank.GetAllBalances(sctx, Address()).Equal(expected) {
		return types.ErrBalanceMismatch
	}

	eTotals, err := k.totals.Get(ctx)
	if err != nil {
		return err
	}

	bTotals, err := k.totalsCurr.Get(ctx)
	if err != nil {
		return err
	}

	if !eTotals.Equal(bTotals) {
		epoch, err := k.getEpochID(sctx)
		if err != nil {
			return err
		}

		usage, err := k.refundUsage.Get(ctx)
		if err != nil {
			return err
		}

		usage, err = usage.At(epoch)
		if err != nil {
			return err
		}

		if err = sctx.EventManager().EmitTypedEvent(&types.EventTotalsUpdated{
			PendingAmount:       eTotals.PendingAmount,
			PendingCount:        eTotals.PendingCount,
			MintedAmount:        eTotals.MintedAmount,
			MintedCount:         eTotals.MintedCount,
			VoidedAmount:        eTotals.VoidedAmount,
			VoidedCount:         eTotals.VoidedCount,
			RefundPendingAmount: eTotals.RefundPendingAmount,
			RefundPendingCount:  eTotals.RefundPendingCount,
			RefundedAmount:      eTotals.RefundedAmount,
			RefundedCount:       eTotals.RefundedCount,
			RefundedInEpoch:     usage.Amount,
		}); err != nil {
			return err
		}
	}

	return nil
}
