package testnetify

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
	"github.com/theckman/yacspin"

	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibchost "github.com/cosmos/ibc-go/v4/modules/core/24-host"
	ibccoretypes "github.com/cosmos/ibc-go/v4/modules/core/types"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	ctypes "github.com/akash-network/akash-api/go/node/cert/v1beta3"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta3"

	"github.com/akash-network/node/x/audit"
	"github.com/akash-network/node/x/cert"
	"github.com/akash-network/node/x/deployment"
	"github.com/akash-network/node/x/escrow"
	"github.com/akash-network/node/x/market"
	"github.com/akash-network/node/x/provider"
)

type (
	GenesisValidators []tmtypes.GenesisValidator
	StakingValidators []stakingtypes.Validator
)

type iState interface {
	pack(cdc codec.Codec) error
	unpack(cdc codec.Codec) error
}

func (u GenesisValidators) Len() int {
	return len(u)
}

func (u GenesisValidators) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func (u GenesisValidators) Less(i, j int) bool {
	return u[i].Power < u[j].Power
}

func (u StakingValidators) Len() int {
	return len(u)
}

func (u StakingValidators) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func (u StakingValidators) Less(i, j int) bool {
	return u[i].DelegatorShares.LT(u[j].DelegatorShares)
}

type AuthState struct {
	gstate               map[string]json.RawMessage
	state                authtypes.GenesisState
	once                 sync.Once
	accs                 authtypes.GenesisAccounts
	unusedAccountNumbers []uint64
}

type BankState struct {
	gstate map[string]json.RawMessage
	state  *banktypes.GenesisState
	once   sync.Once
}

type GovState struct {
	gstate map[string]json.RawMessage
	state  *govtypes.GenesisState
	once   sync.Once
}

type IBCState struct {
	gstate map[string]json.RawMessage
	state  *ibccoretypes.GenesisState
	once   sync.Once
}

type StakingState struct {
	gstate map[string]json.RawMessage
	state  *stakingtypes.GenesisState
	once   sync.Once
}

type SlashingState struct {
	gstate map[string]json.RawMessage
	state  *slashingtypes.GenesisState
	once   sync.Once
}

type DistributionState struct {
	gstate map[string]json.RawMessage
	state  *distributiontypes.GenesisState
	once   sync.Once
}

type AuditState struct {
	gstate map[string]json.RawMessage
	state  *atypes.GenesisState
	once   sync.Once
}

type CertState struct {
	gstate map[string]json.RawMessage
	state  *ctypes.GenesisState
	once   sync.Once
}

type DeploymentState struct {
	gstate map[string]json.RawMessage
	state  *dtypes.GenesisState
	once   sync.Once
}

type EscrowState struct {
	gstate map[string]json.RawMessage
	state  *etypes.GenesisState
	once   sync.Once
}

type MarketState struct {
	gstate map[string]json.RawMessage
	state  *mtypes.GenesisState
	once   sync.Once
}

type ProviderState struct {
	gstate map[string]json.RawMessage
	state  *ptypes.GenesisState
	once   sync.Once
}

var (
	_ iState = (*AuthState)(nil)
	_ iState = (*BankState)(nil)
	_ iState = (*DistributionState)(nil)
	_ iState = (*IBCState)(nil)
	_ iState = (*GovState)(nil)
	_ iState = (*StakingState)(nil)
	_ iState = (*SlashingState)(nil)
	_ iState = (*AuditState)(nil)
	_ iState = (*CertState)(nil)
	_ iState = (*DeploymentState)(nil)
	_ iState = (*EscrowState)(nil)
	_ iState = (*MarketState)(nil)
	_ iState = (*ProviderState)(nil)
)

type GenesisState struct {
	doc   *tmtypes.GenesisDoc
	state map[string]json.RawMessage

	app struct {
		AuthState
		BankState
		GovState
		IBCState
		StakingState
		SlashingState
		DistributionState
		AuditState
		CertState
		DeploymentState
		EscrowState
		MarketState
		ProviderState
	}

	moduleAddresses struct {
		bondedPool    sdk.AccAddress
		notBondedPool sdk.AccAddress
		distribution  sdk.AccAddress
	}
}

func NewGenesisState(sp *yacspin.Spinner, state map[string]json.RawMessage, doc *tmtypes.GenesisDoc) (*GenesisState, error) {
	st := &GenesisState{
		doc:   doc,
		state: state,
	}

	st.app.AuthState.gstate = state
	st.app.BankState.gstate = state
	st.app.GovState.gstate = state
	st.app.IBCState.gstate = state
	st.app.StakingState.gstate = state
	st.app.SlashingState.gstate = state
	st.app.DistributionState.gstate = state
	st.app.AuditState.gstate = state
	st.app.CertState.gstate = state
	st.app.DeploymentState.gstate = state
	st.app.EscrowState.gstate = state
	st.app.MarketState.gstate = state
	st.app.ProviderState.gstate = state

	sp.Message("lookup pool addresses")
	sp.StopMessage("identified modules addresses")
	_ = sp.Start()

	var err error
	st.moduleAddresses.bondedPool, err = st.findModuleAccount(cdc, stakingtypes.BondedPoolName)
	if err != nil {
		return nil, fmt.Errorf("couldn't find bonded_tokens_pool account") // nolint: goerr113
	}

	st.moduleAddresses.notBondedPool, err = st.findModuleAccount(cdc, stakingtypes.NotBondedPoolName)
	if err != nil {
		return nil, fmt.Errorf("couldn't find not_bonded_tokens_pool account") // nolint: goerr113
	}

	st.moduleAddresses.distribution, err = st.findModuleAccount(cdc, "distribution")
	if err != nil {
		return nil, fmt.Errorf("couldn't find distribution account") // nolint: goerr113
	}

	if err = st.app.BankState.unpack(cdc); err != nil {
		return nil, err
	}

	if err = st.app.StakingState.unpack(cdc); err != nil {
		return nil, err
	}

	if err = st.validateBalances(); err != nil {
		return nil, err
	}

	_ = sp.Stop()

	return st, nil
}

func (ga *GenesisState) validateBalances() error {
	bondedTokens := sdk.ZeroInt()
	notBondedTokens := sdk.ZeroInt()

	for _, val := range ga.app.StakingState.state.Validators {
		switch val.GetStatus() {
		case stakingtypes.Bonded:
			bondedTokens = bondedTokens.Add(val.GetTokens())
		case stakingtypes.Unbonding, stakingtypes.Unbonded:
			notBondedTokens = notBondedTokens.Add(val.GetTokens())
		default:
			return fmt.Errorf("invalid validator status") // nolint: goerr113
		}
	}

	for _, ubd := range ga.app.StakingState.state.UnbondingDelegations {
		for _, entry := range ubd.Entries {
			notBondedTokens = notBondedTokens.Add(entry.Balance)
		}
	}

	bondedCoins := sdk.NewCoins(sdk.NewCoin(ga.app.StakingState.state.Params.BondDenom, bondedTokens))
	notBondedCoins := sdk.NewCoins(sdk.NewCoin(ga.app.StakingState.state.Params.BondDenom, notBondedTokens))

	var bondedBalance sdk.Coins
	var notBondedBalance sdk.Coins

	for _, balance := range ga.app.BankState.state.Balances {
		if balance.Address == ga.moduleAddresses.bondedPool.String() {
			bondedBalance = bondedBalance.Add(balance.Coins...)
		}
	}

	for _, balance := range ga.app.BankState.state.Balances {
		if balance.Address == ga.moduleAddresses.notBondedPool.String() {
			notBondedBalance = notBondedBalance.Add(balance.Coins...)
		}
	}

	bondedBalance.Sort()
	notBondedBalance.Sort()

	if !bondedBalance.IsEqual(bondedCoins) {
		return fmt.Errorf("bonded pool balance is different from bonded coins: %s <-> %s", notBondedBalance, notBondedCoins) // nolint: goerr113
	}

	// if !notBondedBalance.IsEqual(notBondedCoins) {
	// 	return fmt.Errorf("not bonded pool balance is different from not bonded coins: %s <-> %s", notBondedBalance, notBondedCoins) // nolint: goerr113
	// }

	return nil
}

func (ga *GenesisState) pack(cdc codec.Codec) error {
	if err := ga.validateBalances(); err != nil {
		return err
	}

	if err := ga.ensureActiveSet(cdc); err != nil {
		return err
	}

	if err := ga.app.AuthState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.BankState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.GovState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.IBCState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.StakingState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.SlashingState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.DistributionState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.AuditState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.CertState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.DeploymentState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.EscrowState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.MarketState.pack(cdc); err != nil {
		return err
	}

	if err := ga.app.ProviderState.pack(cdc); err != nil {
		return err
	}

	state, err := json.Marshal(ga.state)
	if err != nil {
		return err
	}

	ga.doc.AppState = state

	return nil
}

func (ga *AuthState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = authtypes.GetGenesisStateFromAppState(cdc, ga.gstate)
		ga.accs, err = authtypes.UnpackAccounts(ga.state.Accounts)

		if err != nil {
			err = fmt.Errorf("failed to get accounts from any: %s", err.Error()) // nolint: goerr113
		}

		ga.accs = authtypes.SanitizeGenesisAccounts(ga.accs)

		prevAccountNumber := uint64(0)
		for _, acc := range ga.accs {
			diff := acc.GetAccountNumber() - prevAccountNumber
			if diff > 1 {
				accNumber := prevAccountNumber
				for i := uint64(0); i < (diff - 1); i++ {
					accNumber++
					ga.unusedAccountNumbers = append(ga.unusedAccountNumbers, accNumber)
				}
			}

			prevAccountNumber = acc.GetAccountNumber()
		}
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *AuthState) nextAccountNumber() uint64 {
	if len(ga.unusedAccountNumbers) > 0 {
		accNumber := ga.unusedAccountNumbers[0]
		if len(ga.unusedAccountNumbers) > 1 {
			ga.unusedAccountNumbers = ga.unusedAccountNumbers[1:]
		} else {
			ga.unusedAccountNumbers = []uint64{}
		}

		return accNumber
	}

	return ga.accs[len(ga.accs)-1].GetAccountNumber() + 1
}

func (ga *AuthState) pack(cdc codec.Codec) error {
	if len(ga.accs) > 0 {
		ga.accs = authtypes.SanitizeGenesisAccounts(ga.accs)
		var err error
		ga.state.Accounts, err = authtypes.PackAccounts(ga.accs)
		if err != nil {
			return fmt.Errorf("failed to convert accounts into any's: %s", err.Error()) // nolint: goerr113
		}

		stateBz, err := cdc.MarshalJSON(&ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal auth genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[authtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *BankState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = banktypes.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *BankState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		ga.state.Balances = banktypes.SanitizeGenesisBalances(ga.state.Balances)

		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal bank genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[banktypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *GovState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = GetGovGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *GovState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal gov genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[govtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *IBCState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = GetIBCGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *IBCState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal ibc genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[ibchost.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *StakingState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = stakingtypes.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *StakingState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal staking genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[stakingtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *SlashingState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = GetSlashingGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *SlashingState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal staking genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[slashingtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *DistributionState) unpack(cdc codec.Codec) error {
	var err error

	ga.once.Do(func() {
		ga.state = GetDistributionGenesisStateFromAppState(cdc, ga.gstate)
	})

	if err != nil {
		return err
	}

	return nil
}

func (ga *DistributionState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal distribution genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[distributiontypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *AuditState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = audit.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *AuditState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal audit genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[atypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *CertState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = cert.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *CertState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal cert genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[ctypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *DeploymentState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = deployment.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *DeploymentState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal deployment genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[dtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *EscrowState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = escrow.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *EscrowState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal escrow genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[etypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *MarketState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = market.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *MarketState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal market genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[mtypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

// nolint: unused
func (ga *ProviderState) unpack(cdc codec.Codec) error {
	ga.once.Do(func() {
		ga.state = provider.GetGenesisStateFromAppState(cdc, ga.gstate)
	})

	return nil
}

func (ga *ProviderState) pack(cdc codec.Codec) error {
	if ga.state != nil {
		stateBz, err := cdc.MarshalJSON(ga.state)
		if err != nil {
			return fmt.Errorf("failed to marshal provider genesis state: %s", err.Error()) // nolint: goerr113
		}

		ga.gstate[ptypes.ModuleName] = stateBz

		ga.once = sync.Once{}
	}

	return nil
}

func (ga *GenesisState) createCoin(cdc codec.Codec, coin sdk.Coin) error {
	if err := ga.app.BankState.unpack(cdc); err != nil {
		return nil
	}

	supply := ga.app.BankState.state.Supply

	supply = append(supply, coin)

	ga.app.BankState.state.Supply = supply.Sort()

	return nil
}

func (ga *GenesisState) IncreaseSupply(cdc codec.Codec, coins ...sdk.Coin) error {
	if err := ga.app.BankState.unpack(cdc); err != nil {
		return nil
	}

	for _, coin := range coins {
		found := false
		for idx, sCoin := range ga.app.BankState.state.Supply {
			if sCoin.Denom == coin.Denom {
				found = true
				ga.app.BankState.state.Supply[idx] = ga.app.BankState.state.Supply[idx].Add(coin)
				break
			}
		}

		if !found {
			if err := ga.createCoin(cdc, coin); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ga *GenesisState) DecreaseSupply(cdc codec.Codec, coins ...sdk.Coin) error {
	if err := ga.app.BankState.unpack(cdc); err != nil {
		return nil
	}

	for _, coin := range coins {
		found := false
		for idx, sCoin := range ga.app.BankState.state.Supply {
			if sCoin.Denom == coin.Denom {
				found = true
				ga.app.BankState.state.Supply[idx] = ga.app.BankState.state.Supply[idx].Sub(coin)
				break
			}
		}

		if !found {
			return fmt.Errorf("cannot decrease supply for not existing token") // nolint: goerr113
		}
	}

	return nil
}

func (ga *GenesisState) SendFromModuleToModule(cdc codec.Codec, from, to sdk.AccAddress, amt sdk.Coins) error {
	if err := ga.DecreaseBalances(cdc, from, amt); err != nil {
		return err
	}

	if err := ga.IncreaseBalances(cdc, to, amt); err != nil {
		return err
	}

	return nil
}

func (ga *GenesisState) DelegateToPool(cdc codec.Codec, from, to sdk.AccAddress, amt sdk.Coins) error {
	if err := ga.DecreaseBalances(cdc, from, amt); err != nil {
		return err
	}

	if err := ga.IncreaseBalances(cdc, to, amt); err != nil {
		return err
	}

	return nil
}

// IncreaseBalances increases the balance of an account
// and the overall supply of corresponding token by the same amount
func (ga *GenesisState) IncreaseBalances(cdc codec.Codec, addr sdk.AccAddress, coins sdk.Coins) error {
	if err := ga.app.BankState.unpack(cdc); err != nil {
		return nil
	}

	var balance *banktypes.Balance

	for idx := range ga.app.BankState.state.Balances {
		if ga.app.BankState.state.Balances[idx].GetAddress().Equals(addr) {
			balance = &ga.app.BankState.state.Balances[idx]
			break
		}
	}

	if balance == nil {
		return fmt.Errorf("no balances found for account (%s)", addr.String()) // nolint: goerr113
	}

	for _, coin := range coins {
		found := false
		for idx, bCoin := range balance.Coins {
			if bCoin.Denom == coin.Denom {
				found = true
				balance.Coins[idx] = balance.Coins[idx].Add(coin)
				break
			}
		}

		if !found {
			balance.Coins = append(balance.Coins, coin)
		}
	}

	balance.Coins = balance.Coins.Sort()

	return nil
}

func (ga *GenesisState) DecreaseBalances(cdc codec.Codec, addr sdk.AccAddress, coins sdk.Coins) error {
	if err := ga.app.BankState.unpack(cdc); err != nil {
		return nil
	}

	var balance *banktypes.Balance

	for idx := range ga.app.BankState.state.Balances {
		if ga.app.BankState.state.Balances[idx].GetAddress().Equals(addr) {
			balance = &ga.app.BankState.state.Balances[idx]
			break
		}
	}

	if balance == nil {
		return fmt.Errorf("no balances found for account (%s)", addr.String()) // nolint: goerr113
	}

	for _, coin := range coins {
		for idx, bCoin := range balance.Coins {
			if bCoin.Denom == coin.Denom {
				balance.Coins[idx] = balance.Coins[idx].Sub(coin)
				break
			}
		}
	}

	return nil
}

func (ga *GenesisState) IncreaseDelegatorStake(
	cdc codec.Codec,
	addr sdk.AccAddress,
	val sdk.ValAddress,
	coins sdk.Coins,
) error {
	if err := ga.app.StakingState.unpack(cdc); err != nil {
		return err
	}

	if err := ga.app.DistributionState.unpack(cdc); err != nil {
		return err
	}

	var info *distributiontypes.DelegatorStartingInfoRecord
	var delegation *stakingtypes.Delegation
	var sVal *stakingtypes.Validator

	for idx, vl := range ga.app.StakingState.state.Validators {
		if vl.OperatorAddress == val.String() {
			sVal = &ga.app.StakingState.state.Validators[idx]
		}
	}

	if sVal == nil {
		return fmt.Errorf("staking validator (%s) does not exists", val.String()) // nolint: goerr113
	}

	for idx, d := range ga.app.StakingState.state.Delegations {
		if d.DelegatorAddress == addr.String() && d.ValidatorAddress == val.String() {
			delegation = &ga.app.StakingState.state.Delegations[idx]
			break
		}
	}

	if delegation == nil {
		ga.app.StakingState.state.Delegations = append(ga.app.StakingState.state.Delegations, stakingtypes.Delegation{
			DelegatorAddress: addr.String(),
			ValidatorAddress: val.String(),
		})

		delegation = &ga.app.StakingState.state.Delegations[len(ga.app.StakingState.state.Delegations)-1]
	}

	for idx, inf := range ga.app.DistributionState.state.DelegatorStartingInfos {
		if inf.DelegatorAddress == addr.String() && inf.ValidatorAddress == val.String() {
			info = &ga.app.DistributionState.state.DelegatorStartingInfos[idx]
			break
		}
	}

	if info == nil {
		ga.app.DistributionState.state.DelegatorStartingInfos = append(ga.app.DistributionState.state.DelegatorStartingInfos,
			distributiontypes.DelegatorStartingInfoRecord{
				DelegatorAddress: addr.String(),
				ValidatorAddress: val.String(),
				StartingInfo: distributiontypes.DelegatorStartingInfo{
					PreviousPeriod: uint64(ga.doc.InitialHeight - 2),
					Height:         uint64(ga.doc.InitialHeight),
				},
			})

		info = &ga.app.DistributionState.state.DelegatorStartingInfos[len(ga.app.DistributionState.state.DelegatorStartingInfos)-1]
	}

	stake := sdk.NewDec(0)

	for _, coin := range coins {
		stake = stake.Add(coin.Amount.ToDec())
		*sVal, _ = sVal.AddTokensFromDel(coin.Amount)
	}

	info.StartingInfo.Stake = stake
	delegation.Shares = stake

	var err error
	if sVal.IsBonded() {
		err = ga.DelegateToPool(cdc, addr, ga.moduleAddresses.bondedPool, coins)
	} else {
		err = ga.DelegateToPool(cdc, addr, ga.moduleAddresses.notBondedPool, coins)
	}

	if err != nil {
		return err
	}

	if err = ga.sortValidatorsByShares(); err != nil {
		return err
	}

	return nil
}

func (ga *GenesisState) sortValidatorsByShares() error {
	sVal := StakingValidators(ga.app.StakingState.state.Validators)

	sort.Sort(sort.Reverse(sVal))

	ga.app.StakingState.state.Validators = sVal

	return nil
}

func (ga *GenesisState) ensureActiveSet(cdc codec.Codec) error {
	if err := ga.app.StakingState.unpack(cdc); err != nil {
		return err
	}

	sVals := ga.app.StakingState.state.Validators

	vCount := ga.app.StakingState.state.Params.MaxValidators
	if vCount > uint32(len(sVals)) {
		vCount = uint32(len(sVals))
	}

	vals := make([]tmtypes.GenesisValidator, 0, vCount)
	sPowers := make([]stakingtypes.LastValidatorPower, 0, vCount)

	totalPower := int64(0)

	for i, val := range sVals {
		coins := sdk.NewCoins(sdk.NewCoin(ga.app.StakingState.state.Params.BondDenom, val.Tokens))

		if uint32(len(vals)) < vCount {
			if val.IsJailed() {
				continue
			}

			if !val.IsBonded() {
				sVals[i].Status = stakingtypes.Bonded

				err := ga.SendFromModuleToModule(cdc, ga.moduleAddresses.notBondedPool, ga.moduleAddresses.bondedPool, coins)
				if err != nil {
					return err
				}
			}

			pubkey, _ := val.ConsPubKey()

			tmPk, err := cryptocodec.ToTmPubKeyInterface(pubkey)
			if err != nil {
				return err
			}

			power := val.GetDelegatorShares().QuoInt64(denomDecimalPlaces).RoundInt64()
			totalPower += power

			vals = append(vals, tmtypes.GenesisValidator{
				Address: tmPk.Address(),
				PubKey:  tmPk,
				Power:   power,
				Name:    val.Description.Moniker,
			})

			sPowers = append(sPowers, stakingtypes.LastValidatorPower{
				Address: val.OperatorAddress,
				Power:   power,
			})
		} else if val.IsBonded() {
			sVals[i].Status = stakingtypes.Unbonding
			sVals[i].UnbondingHeight = ga.doc.InitialHeight
			err := ga.SendFromModuleToModule(cdc, ga.moduleAddresses.bondedPool, ga.moduleAddresses.notBondedPool, coins)
			if err != nil {
				return err
			}
		}
	}

	ga.app.StakingState.state.LastTotalPower = sdk.NewInt(totalPower)
	ga.app.StakingState.state.LastValidatorPowers = sPowers

	sort.Sort(sort.Reverse(GenesisValidators(vals)))

	ga.doc.Validators = vals

	return nil
}

func (ga *GenesisState) findModuleAccount(cdc codec.Codec, name string) (sdk.AccAddress, error) {
	if err := ga.app.AuthState.unpack(cdc); err != nil {
		return nil, err
	}

	var addr sdk.AccAddress
	for _, acc := range ga.app.AuthState.accs {
		macc, valid := acc.(authtypes.ModuleAccountI)
		if !valid {
			continue
		}

		if macc.GetName() == name {
			addr = macc.GetAddress()
			break
		}
	}

	return addr, nil
}

func (ga *GenesisState) AddNewAccount(cdc codec.Codec, addr sdk.AccAddress, pubkey cryptotypes.PubKey) error {
	if err := ga.app.AuthState.unpack(cdc); err != nil {
		return err
	}

	if err := ga.app.BankState.unpack(cdc); err != nil {
		return err
	}

	if ga.app.AuthState.accs.Contains(addr) {
		return fmt.Errorf("account (%s) already exists", addr.String()) // nolint: goerr113
	}

	genAccount := authtypes.NewBaseAccount(addr, pubkey, ga.app.AuthState.nextAccountNumber(), 0)

	if err := genAccount.Validate(); err != nil {
		return fmt.Errorf("failed to validate new genesis account: %s", err.Error()) // nolint: goerr113
	}

	ga.app.AuthState.accs = append(ga.app.AuthState.accs, genAccount)
	ga.app.AuthState.accs = authtypes.SanitizeGenesisAccounts(ga.app.AuthState.accs)

	ga.app.BankState.state.Balances = append(ga.app.BankState.state.Balances,
		banktypes.Balance{
			Address: addr.String(),
			Coins:   sdk.Coins{},
		},
	)

	return nil
}

func (ga *GenesisState) AddNewValidator(
	cdc codec.Codec,
	addr sdk.ValAddress,
	pk cryptotypes.PubKey,
	name string,
	rates stakingtypes.CommissionRates,
) error {
	if err := ga.app.StakingState.unpack(cdc); err != nil {
		return err
	}

	if err := ga.app.SlashingState.unpack(cdc); err != nil {
		return err
	}

	if err := ga.app.DistributionState.unpack(cdc); err != nil {
		return err
	}

	pkAny, err := codectypes.NewAnyWithValue(pk)
	if err != nil {
		return err
	}

	ga.app.StakingState.state.Validators = append(ga.app.StakingState.state.Validators, stakingtypes.Validator{
		OperatorAddress: addr.String(),
		ConsensusPubkey: pkAny,
		Jailed:          false,
		Status:          stakingtypes.Unbonded,
		Tokens:          sdk.NewInt(0),
		DelegatorShares: sdk.NewDec(0),
		Description: stakingtypes.Description{
			Moniker: name,
		},
		Commission: stakingtypes.Commission{
			CommissionRates: rates,
			UpdateTime:      time.Now().UTC(),
		},
		MinSelfDelegation: sdk.NewInt(1),
	})

	ga.app.DistributionState.state.ValidatorHistoricalRewards = append(ga.app.DistributionState.state.ValidatorHistoricalRewards,
		distributiontypes.ValidatorHistoricalRewardsRecord{
			ValidatorAddress: addr.String(),
			Period:           uint64(ga.doc.InitialHeight - 2),
			Rewards: distributiontypes.ValidatorHistoricalRewards{
				CumulativeRewardRatio: sdk.DecCoins{},
				ReferenceCount:        2,
			},
		})

	ga.app.DistributionState.state.ValidatorCurrentRewards = append(ga.app.DistributionState.state.ValidatorCurrentRewards,
		distributiontypes.ValidatorCurrentRewardsRecord{
			ValidatorAddress: addr.String(),
			Rewards: distributiontypes.ValidatorCurrentRewards{
				Rewards: sdk.DecCoins{},
				Period:  uint64(ga.doc.InitialHeight - 1),
			},
		})

	ga.app.SlashingState.state.SigningInfos = append(ga.app.SlashingState.state.SigningInfos,
		slashingtypes.SigningInfo{
			Address: sdk.ConsAddress(addr).String(),
			ValidatorSigningInfo: slashingtypes.ValidatorSigningInfo{
				Address:             sdk.ConsAddress(addr).String(),
				StartHeight:         ga.doc.InitialHeight - 3,
				IndexOffset:         0,
				JailedUntil:         time.Time{},
				Tombstoned:          false,
				MissedBlocksCounter: 0,
			},
		})
	return nil
}
