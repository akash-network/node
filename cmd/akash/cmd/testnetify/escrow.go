package testnetify

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"

	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
)

func (ga *GenesisState) modifyEscrowState(cdc codec.Codec, cfg *EscrowConfig) error {
	if cfg == nil {
		return nil
	}

	if err := ga.app.EscrowState.unpack(cdc); err != nil {
		return err
	}

	amap := make(map[etypes.AccountID]etypes.Account, len(ga.app.EscrowState.state.Accounts))

	for _, acc := range ga.app.EscrowState.state.Accounts {
		amap[acc.ID] = acc
	}

	if cfg.PatchDanglingPayments {
		for idx, payment := range ga.app.EscrowState.state.Payments {
			// make sure there's an account
			acc, found := amap[payment.AccountID]
			if !found {
				return fmt.Errorf("%w: no account for payment %s %s (idx %v)", etypes.ErrAccountNotFound, payment.AccountID, payment.PaymentID, idx)
			}

			if ((payment.State == etypes.PaymentOpen) && (acc.State != etypes.AccountOpen)) ||
				((payment.State == etypes.PaymentOverdrawn) && (acc.State != etypes.AccountOverdrawn)) {
				switch acc.State {
				case etypes.AccountOpen:
					ga.app.EscrowState.state.Payments[idx].State = etypes.PaymentOpen
				case etypes.AccountOverdrawn:
					ga.app.EscrowState.state.Payments[idx].State = etypes.PaymentOverdrawn
				case etypes.AccountClosed:
					ga.app.EscrowState.state.Payments[idx].State = etypes.PaymentClosed
				}
			}
		}
	}

	return nil
}
