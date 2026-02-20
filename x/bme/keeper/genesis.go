package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/bme/v1"
)

// InitGenesis initiate genesis state and return updated validator details
func (k *keeper) InitGenesis(ctx sdk.Context, data *types.GenesisState) {
	if err := data.Validate(); err != nil {
		panic(err)
	}
	if err := k.SetParams(ctx, data.Params); err != nil {
		panic(err)
	}

	for _, coin := range data.State.TotalMinted {
		if err := k.totalMinted.Set(ctx, coin.Denom, coin.Amount); err != nil {
			panic(err)
		}
	}

	for _, coin := range data.State.TotalBurned {
		if err := k.totalBurned.Set(ctx, coin.Denom, coin.Amount); err != nil {
			panic(err)
		}
	}

	for _, coin := range data.State.RemintCredits {
		if err := k.remintCredits.Set(ctx, coin.Denom, coin.Amount); err != nil {
			panic(err)
		}
	}

	err := k.status.Set(ctx, types.Status{
		Status:          types.MintStatusHaltCR,
		EpochHeightDiff: data.Params.MinEpochBlocks,
	})
	if err != nil {
		panic(err)
	}

	err = k.mintEpoch.Set(ctx, types.MintEpoch{
		NextEpoch: data.Params.MinEpochBlocks,
	})
	if err != nil {
		panic(err)
	}

	if data.Ledger != nil {
		for _, record := range data.Ledger.Records {
			if err := k.AddLedgerRecord(ctx, record.ID, record.Record); err != nil {
				panic(err)
			}
		}

		for _, record := range data.Ledger.PendingRecords {
			if err := k.AddLedgerPendingRecord(ctx, record.ID, record.Record); err != nil {
				panic(err)
			}
		}
	}
}

// ExportGenesis returns genesis state for the deployment module
func (k *keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	params, err := k.GetParams(ctx)
	if err != nil {
		panic(err)
	}

	state, err := k.GetState(ctx)
	if err != nil {
		panic(err)
	}

	ledgerRecords := make([]types.GenesisLedgerRecord, 0)

	err = k.IterateLedgerRecords(ctx, func(id types.LedgerRecordID, record types.LedgerRecord) (bool, error) {
		ledgerRecords = append(ledgerRecords, types.GenesisLedgerRecord{
			ID:     id,
			Record: record,
		})
		return false, nil
	})
	if err != nil {
		panic(err)
	}

	ledgerPendingRecords := make([]types.GenesisLedgerPendingRecord, 0)
	err = k.IterateLedgerPendingRecords(ctx, func(id types.LedgerRecordID, record types.LedgerPendingRecord) (bool, error) {
		ledgerPendingRecords = append(ledgerPendingRecords, types.GenesisLedgerPendingRecord{
			ID:     id,
			Record: record,
		})

		return false, nil
	})
	if err != nil {
		panic(err)
	}

	return &types.GenesisState{
		Params: params,
		State: types.GenesisVaultState{
			TotalBurned:   state.TotalBurned,
			TotalMinted:   state.TotalMinted,
			RemintCredits: state.RemintCredits,
		},
		Ledger: &types.GenesisLedgerState{
			Records:        ledgerRecords,
			PendingRecords: ledgerPendingRecords,
		},
	}
}
