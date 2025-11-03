package v7

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dv1 "pkg.akt.dev/go/node/deployment/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"

	dimports "pkg.akt.dev/node/v2/x/deployment/imports"
)

type migrationGrant struct {
	grantee  string
	srcGrant sdk.DecCoin
	dstGrant sdk.DecCoin
}

type migrationGrants map[string]*migrationGrant

type Migration struct {
	dk dimports.DeploymentKeeper
	mk dimports.MarketKeeper
	ek dimports.EscrowKeeper
	ak dimports.AuthzKeeper
}

func NewMigration(dk dimports.DeploymentKeeper, mk dimports.MarketKeeper, ek dimports.EscrowKeeper, ak dimports.AuthzKeeper) Migration {
	return Migration{
		dk: dk,
		mk: mk,
		ek: ek,
		ak: ak,
	}
}

func (m Migration) Run(sctx sdk.Context, did dv1.DeploymentID, fromDenom, toDenom string, rate sdkmath.LegacyDec) (sdk.Coin, sdk.Coin, error) {
	if err := m.migrateGroups(sctx, did, fromDenom, toDenom, rate); err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	srcCoin, dstCoin, grants, err := m.migrateEscrow(sctx, did, fromDenom, toDenom, rate)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	err = m.createGrants(sctx, grants)
	if err != nil {
		return sdk.Coin{}, sdk.Coin{}, err
	}

	return srcCoin, dstCoin, nil
}

func (m Migration) migrateGroups(ctx sdk.Context, did dv1.DeploymentID, fromDenom, toDenom string, rate sdkmath.LegacyDec) error {
	groups, err := m.dk.GetGroups(ctx, did)
	if err != nil {
		return err
	}

	for _, group := range groups {
		if group.State != dvbeta.GroupOpen && group.State != dvbeta.GroupPaused {
			continue
		}

		changed := false
		for j := range group.GroupSpec.Resources {
			res := &group.GroupSpec.Resources[j]
			if res.Price.Denom == fromDenom {
				res.Price = convertDecCoin(res.Price, toDenom, rate)
				changed = true
			}
		}

		if changed {
			if err := m.dk.SaveGroup(ctx, group); err != nil {
				return fmt.Errorf("save group %s: %w", group.ID, err)
			}
		}

		// Migrate associated market objects for this group
		if err := m.migrateMarket(ctx, group.ID, fromDenom, toDenom, rate); err != nil {
			return fmt.Errorf("migrate market objects for group %s: %w", group.ID, err)
		}
	}

	return nil
}

func (m Migration) migrateMarket(ctx sdk.Context, gid dv1.GroupID, fromDenom, toDenom string, rate sdkmath.LegacyDec) error {
	// Migrate open and active orders
	for _, state := range []mvbeta.Order_State{mvbeta.OrderOpen, mvbeta.OrderActive} {
		m.mk.WithOrdersForGroup(ctx, gid, state, func(order mvbeta.Order) bool {
			changed := false
			for j := range order.Spec.Resources {
				res := &order.Spec.Resources[j]
				if res.Price.Denom == fromDenom {
					res.Price = convertDecCoin(res.Price, toDenom, rate)
					changed = true
				}
			}

			if changed {
				if err := m.mk.SaveOrder(ctx, order); err != nil {
					return true
				}
			}

			// Migrate bids for this order
			m.migrateBids(ctx, order.ID, fromDenom, toDenom, rate)

			// Migrate lease for this order (active bids have leases)
			m.migrateLease(ctx, order.ID, fromDenom, toDenom, rate)

			return false
		})
	}

	return nil
}

func (m Migration) migrateBids(ctx sdk.Context, oid mv1.OrderID, fromDenom, toDenom string, rate sdkmath.LegacyDec) {
	for _, state := range []mvbeta.Bid_State{mvbeta.BidOpen, mvbeta.BidActive} {
		m.mk.WithBidsForOrder(ctx, oid, state, func(bid mvbeta.Bid) bool {
			changed := false

			if bid.Price.Denom == fromDenom {
				bid.Price = convertDecCoin(bid.Price, toDenom, rate)
				changed = true
			}

			for i := range bid.ResourcesOffer {
				if p := bid.ResourcesOffer[i].Prices; p != nil {
					if p.Cpu != nil && p.Cpu.Denom == fromDenom {
						c := convertDecCoin(*p.Cpu, toDenom, rate)
						p.Cpu = &c
						changed = true
					}
					if p.Memory != nil && p.Memory.Denom == fromDenom {
						c := convertDecCoin(*p.Memory, toDenom, rate)
						p.Memory = &c
						changed = true
					}
					if p.Gpu != nil && p.Gpu.Denom == fromDenom {
						c := convertDecCoin(*p.Gpu, toDenom, rate)
						p.Gpu = &c
						changed = true
					}
					for j := range p.Storage {
						if p.Storage[j].Price != nil && p.Storage[j].Price.Denom == fromDenom {
							c := convertDecCoin(*p.Storage[j].Price, toDenom, rate)
							p.Storage[j].Price = &c
							changed = true
						}
					}
					for j := range p.Endpoints {
						if p.Endpoints[j].Price != nil && p.Endpoints[j].Price.Denom == fromDenom {
							c := convertDecCoin(*p.Endpoints[j].Price, toDenom, rate)
							p.Endpoints[j].Price = &c
							changed = true
						}
					}
				}
			}

			if changed {
				_ = m.mk.SaveBid(ctx, bid)
			}
			return false
		})
	}
}

func (m Migration) migrateLease(ctx sdk.Context, oid mv1.OrderID, fromDenom, toDenom string, rate sdkmath.LegacyDec) {
	for _, bidState := range []mvbeta.Bid_State{mvbeta.BidActive} {
		lease, found := m.mk.LeaseForOrder(ctx, bidState, oid)
		if !found {
			continue
		}

		if lease.State != mv1.LeaseActive && lease.State != mv1.LeaseInsufficientFunds {
			continue
		}

		if lease.Price.Denom == fromDenom {
			lease.Price = convertDecCoin(lease.Price, toDenom, rate)
			_ = m.mk.SaveLease(ctx, lease)
		}
	}
}

func (m Migration) migrateEscrow(
	ctx sdk.Context,
	did dv1.DeploymentID,
	fromDenom, toDenom string,
	rate sdkmath.LegacyDec,
) (sdk.Coin, sdk.Coin, migrationGrants, error) {
	accountID := did.ToEscrowAccountID()

	acc, err := m.ek.GetAccount(ctx, accountID)
	if err != nil {
		// Account may not exist (e.g. already closed)
		return sdk.Coin{}, sdk.Coin{}, nil, err
	}

	if acc.State.State != etypes.StateOpen && acc.State.State != etypes.StateOverdrawn {
		return sdk.Coin{}, sdk.Coin{}, nil, err
	}

	srcCoin := sdk.NewDecCoin(fromDenom, sdkmath.ZeroInt())
	dstCoin := sdk.NewDecCoin(toDenom, sdkmath.ZeroInt())

	// Migrate Funds
	for i := range acc.State.Funds {
		f := &acc.State.Funds[i]
		if f.Denom == fromDenom {
			f.Denom = toDenom

			// if balance is negative then an account is overdrawn, convert only denom and amount, but do not
			// include amounts for further burn/mint
			if f.Amount.GTE(sdkmath.LegacyZeroDec()) {
				srcCoin = srcCoin.Add(sdk.NewDecCoinFromDec(fromDenom, f.Amount))

				f.Amount.MulMut(rate)

				dstCoin = dstCoin.Add(sdk.NewDecCoinFromDec(toDenom, f.Amount))
			} else {
				f.Amount.MulMut(rate)
			}
		}
	}

	grants := make(migrationGrants)

	// Migrate Deposits and track grant totals
	for i := range acc.State.Deposits {
		d := &acc.State.Deposits[i]
		if d.Balance.Denom == fromDenom {
			srcBalance := sdk.NewDecCoin(fromDenom, sdkmath.ZeroInt())
			dstBalance := sdk.NewDecCoin(toDenom, sdkmath.ZeroInt())

			srcBalance.Amount.AddMut(d.Balance.Amount)

			d.Balance.Denom = toDenom
			d.Balance.Amount.MulMut(rate)

			dstBalance.Amount.AddMut(d.Balance.Amount)

			if d.Source == deposit.SourceGrant {
				grant, exist := grants[d.Owner]
				if !exist {
					grant = &migrationGrant{
						grantee:  acc.State.Owner,
						srcGrant: srcBalance,
						dstGrant: dstBalance,
					}
					grants[d.Owner] = grant
				}

			}
		}
	}

	acc.State.Transferred = append(acc.State.Transferred, sdk.NewDecCoin(toDenom, sdkmath.ZeroInt()))

	if err = m.ek.SaveAccountRaw(ctx, acc); err != nil {
		return sdk.Coin{}, sdk.Coin{}, nil, err
	}

	// Migrate payments
	payments := m.ek.GetAccountPayments(ctx, accountID, []etypes.State{etypes.StateOpen, etypes.StateOverdrawn})
	for _, pmnt := range payments {
		changed := false

		if pmnt.State.Rate.Denom == fromDenom {
			pmnt.State.Rate = convertDecCoin(pmnt.State.Rate, toDenom, rate)
			changed = true
		}
		if pmnt.State.Balance.Denom == fromDenom {
			pmnt.State.Balance = convertDecCoin(pmnt.State.Balance, toDenom, rate)
			changed = true
		}
		if pmnt.State.Unsettled.Denom == fromDenom {
			pmnt.State.Unsettled = convertDecCoin(pmnt.State.Unsettled, toDenom, rate)
			changed = true
		}
		if pmnt.State.Withdrawn.Denom == fromDenom {
			pmnt.State.Withdrawn = convertCoin(pmnt.State.Withdrawn, toDenom, rate)
			changed = true
		}

		if changed {
			if err := m.ek.SavePaymentRaw(ctx, pmnt); err != nil {
				return sdk.Coin{}, sdk.Coin{}, nil, err
			}
		}
	}

	sCoin, srcCoin := srcCoin.TruncateDecimal()
	dCoin, dstCoin := dstCoin.TruncateDecimal()

	return sCoin, dCoin, grants, nil
}

func (m Migration) createGrants(ctx sdk.Context, grants migrationGrants) error {
	msgTypeURL := (&ev1.DepositAuthorization{}).MsgTypeURL()

	for key, grant := range grants {
		granter, err := sdk.AccAddressFromBech32(key)
		if err != nil {
			return fmt.Errorf("parse granter address: %w", err)
		}

		grantee, err := sdk.AccAddressFromBech32(grant.grantee)
		if err != nil {
			return fmt.Errorf("parse grantee address: %w", err)
		}

		var spendLimits sdk.Coins

		// only existing grants are being updated
		existing, expiration := m.ak.GetAuthorization(ctx, grantee, granter, msgTypeURL)
		if existing != nil {
			da, _ := existing.(*ev1.DepositAuthorization)

			spendLimits = da.SpendLimits

			if da.SpendLimit.Amount.GT(sdkmath.ZeroInt()) {
				spendLimits.Add(da.SpendLimit)
			}
		}

		dCoin, _ := grant.dstGrant.TruncateDecimal()

		spendLimits = spendLimits.Add(dCoin)

		auth := ev1.NewDepositAuthorization(
			ev1.DepositAuthorizationScopes{ev1.DepositScopeDeployment},
			spendLimits,
		)

		if err = m.ak.SaveGrant(ctx, grantee, granter, auth, expiration); err != nil {
			return fmt.Errorf("save grant: %w", err)
		}
	}

	return nil
}

// DetectDenom returns the denom used in the first resource price of the first group.
func DetectDenom(groups dvbeta.Groups) string {
	for _, g := range groups {
		for _, r := range g.GroupSpec.Resources {
			if !r.Price.IsZero() {
				return r.Price.Denom
			}
		}
	}
	return ""
}

func convertDecCoin(c sdk.DecCoin, toDenom string, rate sdkmath.LegacyDec) sdk.DecCoin {
	return sdk.NewDecCoinFromDec(toDenom, c.Amount.Mul(rate))
}

func convertCoin(c sdk.Coin, toDenom string, rate sdkmath.LegacyDec) sdk.Coin {
	newAmount := rate.MulInt(c.Amount).TruncateInt()
	return sdk.NewCoin(toDenom, newAmount)
}
