// Package v0_15_0
// nolint revive
package v0_15_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	ev1beta1 "github.com/akash-network/akash-api/go/node/escrow/v1beta1"
	ev1beta2 "github.com/akash-network/akash-api/go/node/escrow/v1beta2"

	utypes "github.com/akash-network/node/upgrades/types"
)

type escrowMigrations struct {
	utypes.Migrator
}

func newEscrowMigration(m utypes.Migrator) utypes.Migration {
	return escrowMigrations{Migrator: m}
}

func (m escrowMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 1 to 2.
func (m escrowMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	err := utypes.MigrateValue(store, m.Codec(), ev1beta2.AccountKeyPrefix(), migrateAccount)
	if err != nil {
		return err
	}

	err = utypes.MigrateValue(store, m.Codec(), ev1beta2.PaymentKeyPrefix(), migratePayment)
	if err != nil {
		return err
	}

	return nil
}

func migrateAccount(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var from ev1beta1.Account
	cdc.MustUnmarshal(fromBz, &from)
	to := ev1beta2.Account{
		ID: ev1beta2.AccountID{
			Scope: from.ID.Scope,
			XID:   from.ID.XID,
		},
		Owner:       from.Owner,
		State:       ev1beta2.Account_State(from.State),
		Balance:     sdk.NewDecCoinFromCoin(from.Balance),
		Transferred: sdk.NewDecCoinFromCoin(from.Transferred),
		SettledAt:   from.SettledAt,
		// Correctly initialize the new fields
		// - Account.Depositor as Account.Owner
		// - Account.Funds as a DecCoin of zero value
		Depositor: from.Owner,
		Funds:     sdk.NewDecCoin(from.Balance.Denom, sdk.ZeroInt()),
	}

	return &to
}

func migratePayment(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var from ev1beta1.Payment
	cdc.MustUnmarshal(fromBz, &from)

	return &ev1beta2.FractionalPayment{
		AccountID: ev1beta2.AccountID{
			Scope: from.AccountID.Scope,
			XID:   from.AccountID.XID,
		},
		PaymentID: from.PaymentID,
		Owner:     from.Owner,
		State:     ev1beta2.FractionalPayment_State(from.State),
		Rate:      sdk.NewDecCoinFromCoin(from.Rate),
		Balance:   sdk.NewDecCoinFromCoin(from.Balance),
		Withdrawn: from.Withdrawn,
	}
}
