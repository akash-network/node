package types

import (
	"errors"
	"fmt"
	"reflect"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	capabilitykeeper "github.com/cosmos/cosmos-sdk/x/capability/keeper"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ibctransferkeeper "github.com/cosmos/ibc-go/v3/modules/apps/transfer/keeper"
	ibckeeper "github.com/cosmos/ibc-go/v3/modules/core/keeper"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/akash-network/node/x/audit"
	"github.com/akash-network/node/x/cert"
	dkeeper "github.com/akash-network/node/x/deployment/keeper"
	escrowkeeper "github.com/akash-network/node/x/escrow/keeper"
	agovkeeper "github.com/akash-network/node/x/gov/keeper"
	"github.com/akash-network/node/x/inflation"
	mkeeper "github.com/akash-network/node/x/market/keeper"
	pkeeper "github.com/akash-network/node/x/provider/keeper"
	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
)

var (
	upgrades = map[string]UpgradeInitFn{}
	forks    = map[int64]IFork{}
)

var (
	ErrEmptyFieldName = errors.New("empty field name")
)

// IUpgrade defines an interface to run a SoftwareUpgradeProposal
type IUpgrade interface {
	StoreLoader() *storetypes.StoreUpgrades
	UpgradeHandler() upgradetypes.UpgradeHandler
}

// IFork defines an interface for a non-software upgrade proposal Hard Fork at a given height to implement.
// There is one time code that can be added for the start of the Fork, in `BeginForkLogic`.
// Any other change in the code should be height-gated, if the goal is to have old and new binaries
// to be compatible prior to the upgrade height.
type IFork interface {
	Name() string
	BeginForkLogic(sdk.Context, *AppKeepers)
}

type UpgradeInitFn func(log.Logger, *App) (IUpgrade, error)

type AppKeepers struct {
	Cosmos struct {
		Acct                 authkeeper.AccountKeeper
		Authz                authzkeeper.Keeper
		Bank                 bankkeeper.Keeper
		Cap                  *capabilitykeeper.Keeper
		Staking              stakingkeeper.Keeper
		Slashing             slashingkeeper.Keeper
		Mint                 mintkeeper.Keeper
		Distr                distrkeeper.Keeper
		Gov                  govkeeper.Keeper
		Crisis               crisiskeeper.Keeper
		Upgrade              upgradekeeper.Keeper
		Params               paramskeeper.Keeper
		IBC                  *ibckeeper.Keeper
		Evidence             evidencekeeper.Keeper
		Transfer             ibctransferkeeper.Keeper
		ScopedIBCKeeper      capabilitykeeper.ScopedKeeper
		ScopedTransferKeeper capabilitykeeper.ScopedKeeper
	}

	Akash struct {
		Escrow     escrowkeeper.Keeper
		Deployment dkeeper.IKeeper
		Market     mkeeper.IKeeper
		Provider   pkeeper.IKeeper
		Audit      audit.Keeper
		Cert       cert.Keeper
		Inflation  inflation.Keeper
		Staking    astakingkeeper.IKeeper
		Gov        agovkeeper.IKeeper
	}
}

type App struct {
	Keepers      AppKeepers
	Configurator module.Configurator
	MM           *module.Manager
}

func RegisterUpgrade(name string, fn UpgradeInitFn) {
	if _, exists := upgrades[name]; exists {
		panic(fmt.Sprintf("upgrade \"%s\" already registered", name))
	}

	upgrades[name] = fn
}

func RegisterFork(height int64, fork IFork) {
	if _, exists := forks[height]; exists {
		panic(fmt.Sprintf("fork \"%s\" for height %d already registered", fork.Name(), height))
	}

	forks[height] = fork
}

func GetUpgradesList() map[string]UpgradeInitFn {
	return upgrades
}

func GetForksList() map[int64]IFork {
	return forks
}

// FindStructField if an interface is either a struct or a pointer to a struct
// and has the defined member field, if error is nil, the given
// fieldName exists and is accessible with reflect.
func FindStructField[C any](obj interface{}, fieldName string) (C, error) {
	if fieldName == "" {
		return *new(C), ErrEmptyFieldName
	}
	rValue := reflect.ValueOf(obj)

	if rValue.Type().Kind() != reflect.Ptr {
		pValue := reflect.New(reflect.TypeOf(obj))
		pValue.Elem().Set(rValue)
		rValue = pValue
	}

	field := rValue.Elem().FieldByName(fieldName)
	if !field.IsValid() {
		return *new(C), fmt.Errorf("interface `%s` does not have the field `%s`", // nolint: goerr113
			rValue.Type(),
			fieldName)
	}

	res, valid := field.Interface().(C)
	if !valid {
		return *new(C), fmt.Errorf( // nolint: goerr113
			"object's `%s` expected type `%s` does not match actual `%s`",
			fieldName,
			reflect.TypeOf(*new(C)), field.Type().String())
	}

	return res, nil
}
