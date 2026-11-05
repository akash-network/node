package v2_2_0_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/testutil"

	repair "pkg.akt.dev/node/v2/upgrades/software/v2.2.0"

	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/escrow"
	ekeeper "pkg.akt.dev/node/v2/x/escrow/keeper"
)

func dec(amount string) sdk.DecCoin {
	return sdk.NewDecCoinFromDec("uact", sdkmath.LegacyMustNewDecFromStr(amount))
}

func payment(id mv1.LeaseID, status etypes.State) etypes.Payment {
	return etypes.Payment{ID: id.ToEscrowPaymentID(), State: etypes.PaymentState{
		Owner: id.Provider, State: status, Rate: dec("2.5"), Balance: dec("13.75"),
		Unsettled: dec("4.25"), Withdrawn: sdk.NewInt64Coin("uact", 17),
	}}
}

func snapshot(t *testing.T, store storetypes.KVStore) map[string]string {
	t.Helper()
	result := map[string]string{}
	iter := store.Iterator(nil, nil)
	for ; iter.Valid(); iter.Next() {
		result[string(iter.Key())] = string(iter.Value())
	}
	require.NoError(t, iter.Close())
	return result
}

func TestRepairSelectionAndPreservation(t *testing.T) {
	cases := []struct {
		name     string
		lease    mv1.Lease_State
		state    etypes.State
		parent   etypes.State
		repaired bool
	}{
		{"closed lease", mv1.LeaseClosed, etypes.StateOpen, etypes.StateOpen, true},
		{"insufficient funds lease", mv1.LeaseInsufficientFunds, etypes.StateOpen, etypes.StateOverdrawn, true},
		{"already refunded parent", mv1.LeaseClosed, etypes.StateOpen, etypes.StateClosed, true},
		{"missing parent", mv1.LeaseClosed, etypes.StateOpen, etypes.StateInvalid, true},
		{"active lease", mv1.LeaseActive, etypes.StateOpen, etypes.StateOpen, false},
		{"reclaiming lease", mv1.LeaseReclaiming, etypes.StateOpen, etypes.StateOpen, false},
		{"legitimate debt", mv1.LeaseInsufficientFunds, etypes.StateOverdrawn, etypes.StateOverdrawn, false},
		{"closed payment", mv1.LeaseClosed, etypes.StateClosed, etypes.StateOpen, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := state.SetupTestSuite(t)
			ctx := s.Context().WithBlockHeight(100)
			k := s.EscrowKeeper()
			m := s.MarketKeeper()
			id := testutil.LeaseID(t)
			p := payment(id, tc.state)
			require.NoError(t, k.SavePaymentRaw(ctx, p))

			require.NoError(t, m.SaveLease(ctx, mv1.Lease{ID: id, State: tc.lease, Price: p.State.Rate, CreatedAt: 1, ClosedOn: 20}))

			if tc.parent != etypes.StateInvalid {
				// Preserve multiple denoms, separate depositors, prior transfers and even a
				// future clock verbatim. The repair must never infer a financial correction.
				a := etypes.Account{ID: p.ID.AID, State: etypes.AccountState{Owner: id.Owner, State: tc.parent, SettledAt: 150,
					Funds:       []etypes.Balance{{Denom: "uact", Amount: sdkmath.LegacyNewDec(1000)}, {Denom: "uakt", Amount: sdkmath.LegacyNewDec(500)}},
					Transferred: sdk.NewDecCoins(dec("17")),
					Deposits:    []etypes.Depositor{{Owner: id.Owner, Height: 3, Balance: dec("700")}, {Owner: id.Provider, Height: 5, Balance: dec("300")}},
				}}
				if tc.parent == etypes.StateClosed {
					a.State.Funds = nil
					a.State.Deposits = nil
				}
				require.NoError(t, k.SaveAccountRaw(ctx, a))
			}
			hookCalls := 0
			k.AddOnPaymentClosedHook(func(sdk.Context, etypes.Payment) error { hookCalls++; return nil })
			store := ctx.KVStore(k.StoreKey())
			before := snapshot(t, store)
			banksBefore := len(s.BankKeeper().Calls)
			eventsBefore := len(ctx.EventManager().Events())
			result, err := repair.RepairEscrowPayments(ctx, k, m)
			require.NoError(t, err)
			expected := before
			if tc.repaired {
				require.EqualValues(t, 1, result.ClosedPayments)
				delete(expected, string(ekeeper.BuildPaymentsKey(etypes.StateOpen, &p.ID)))
				p.State.State = etypes.StateClosed
				expected[string(ekeeper.BuildPaymentsKey(etypes.StateClosed, &p.ID))] = string(k.Codec().MustMarshal(&p.State))
			} else {
				require.Zero(t, result.ClosedPayments)
			}

			require.Equal(t, expected, snapshot(t, store), "only the selected payment's state and index may change")
			got, err := k.GetPayment(ctx, p.ID)
			require.NoError(t, err)
			require.Equal(t, p, got)
			require.Zero(t, hookCalls)
			require.Len(t, s.BankKeeper().Calls, banksBefore)
			require.Len(t, ctx.EventManager().Events(), eventsBefore)
			again, err := repair.RepairEscrowPayments(ctx, k, m)
			require.NoError(t, err)
			require.Zero(t, again.ClosedPayments)
			require.Equal(t, expected, snapshot(t, store), "a repeated repair must be a byte-for-byte no-op")
		})
	}
}

func TestRepairErrorsAreAtomic(t *testing.T) {
	for _, kind := range []string{"duplicate closed", "duplicate overdrawn", "wrong state", "invalid bytes", "short key", "bad lease ID"} {
		t.Run(kind, func(t *testing.T) {
			s := state.SetupTestSuite(t)
			ctx := s.Context()
			k := s.EscrowKeeper()
			m := s.MarketKeeper()
			store := ctx.KVStore(k.StoreKey())
			id := testutil.LeaseID(t)
			id.DSeq = 1
			good := payment(id, etypes.StateOpen)
			require.NoError(t, k.SavePaymentRaw(ctx, good))
			require.NoError(t, m.SaveLease(ctx, mv1.Lease{ID: id, State: mv1.LeaseClosed, Price: good.State.Rate}))
			id.DSeq = 9
			bad := payment(id, etypes.StateOpen)
			require.NoError(t, k.SavePaymentRaw(ctx, bad))
			require.NoError(t, m.SaveLease(ctx, mv1.Lease{ID: id, State: mv1.LeaseClosed, Price: bad.State.Rate}))
			key := ekeeper.BuildPaymentsKey(etypes.StateOpen, &bad.ID)
			switch kind {
			case "duplicate closed":
				bad.State.State = etypes.StateClosed
				require.NoError(t, k.SavePaymentRaw(ctx, bad))
			case "duplicate overdrawn":
				bad.State.State = etypes.StateOverdrawn
				require.NoError(t, k.SavePaymentRaw(ctx, bad))
			case "wrong state":
				bad.State.State = etypes.StateClosed
				store.Set(key, k.Codec().MustMarshal(&bad.State))
			case "invalid bytes":
				store.Set(key, []byte{0xff})
			case "short key":
				store.Set(ekeeper.BuildPaymentsKey(etypes.StateOpen, nil), []byte{1})
			case "bad lease ID":
				bad.ID.XID = "invalid"
				require.NoError(t, k.SavePaymentRaw(ctx, bad))
			}
			before := snapshot(t, store)
			result, err := repair.RepairEscrowPayments(ctx, k, m)
			require.Error(t, err)
			require.Equal(t, repair.EscrowRepairResult{}, result)
			require.Equal(t, before, snapshot(t, store), "an error must not leave a partially repaired store")
		})
	}
}

func TestRepairDoesNotChargeOrphanDuringLaterSettlement(t *testing.T) {
	s := state.SetupTestSuite(t)
	ctx := s.Context().WithBlockHeight(20)
	k := s.EscrowKeeper()
	m := s.MarketKeeper()
	id := testutil.LeaseID(t)
	old := payment(id, etypes.StateOpen)
	require.NoError(t, k.SavePaymentRaw(ctx, old))
	require.NoError(t, m.SaveLease(ctx, mv1.Lease{ID: id, State: mv1.LeaseClosed, Price: old.State.Rate, ClosedOn: 10}))
	id.OSeq++
	live := payment(id, etypes.StateOpen)
	live.State.Balance = dec("0")
	live.State.Unsettled = dec("0")
	live.State.Withdrawn = sdk.NewInt64Coin("uact", 0)
	require.NoError(t, k.SavePaymentRaw(ctx, live))
	require.NoError(t, m.SaveLease(ctx, mv1.Lease{ID: id, State: mv1.LeaseActive, Price: live.State.Rate}))
	a := etypes.Account{ID: old.ID.AID, State: etypes.AccountState{Owner: id.Owner, State: etypes.StateOpen, SettledAt: 10,
		Funds: []etypes.Balance{{Denom: "uact", Amount: sdkmath.LegacyNewDec(1000)}}, Transferred: sdk.DecCoins{dec("0")},
		Deposits: []etypes.Depositor{{Owner: id.Owner, Height: 1, Balance: dec("1000")}}}}
	require.NoError(t, k.SaveAccountRaw(ctx, a))
	_, err := repair.RepairEscrowPayments(ctx, k, m)
	require.NoError(t, err)
	overdrawn, err := k.AccountSettle(ctx, a.ID)
	require.NoError(t, err)
	require.False(t, overdrawn)
	got, err := k.GetAccount(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, int64(20), got.State.SettledAt)
	require.Equal(t, sdkmath.LegacyNewDec(975), got.State.Funds[0].Amount)
	require.Equal(t, sdk.NewDecCoins(dec("25")), got.State.Transferred)
	active, err := k.GetPayment(ctx, live.ID)
	require.NoError(t, err)
	require.Equal(t, dec("25"), active.State.Balance)
	old.State.State = etypes.StateClosed
	closed, err := k.GetPayment(ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, old, closed)
	require.ErrorContains(t, k.PaymentWithdraw(ctx, old.ID), "payment closed")
	// The state survives the standard module export/import path.
	exported := escrow.ExportGenesis(ctx, k)
	other := state.SetupTestSuite(t)
	escrow.InitGenesis(other.Context(), other.EscrowKeeper(), exported)
	require.Equal(t, exported, escrow.ExportGenesis(other.Context(), other.EscrowKeeper()))
}

func TestRepairSkipsOtherScopes(t *testing.T) {
	s := state.SetupTestSuite(t)
	ctx := s.Context()
	k := s.EscrowKeeper()
	p := payment(testutil.LeaseID(t), etypes.StateOpen)
	p.ID.AID = escrowid.Account{Scope: escrowid.ScopeBid, XID: p.ID.AID.XID + "/" + p.ID.XID}
	require.NoError(t, k.SavePaymentRaw(ctx, p))
	before := snapshot(t, ctx.KVStore(k.StoreKey()))
	result, err := repair.RepairEscrowPayments(ctx, k, s.MarketKeeper())
	require.NoError(t, err)
	require.Equal(t, repair.EscrowRepairResult{}, result)
	require.Equal(t, before, snapshot(t, ctx.KVStore(k.StoreKey())))
}
