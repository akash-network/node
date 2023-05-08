package types

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	capabilitykeeper "github.com/cosmos/cosmos-sdk/x/capability/keeper"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	evidencekeeper "github.com/cosmos/cosmos-sdk/x/evidence/keeper"
	feegrantkeeper "github.com/cosmos/cosmos-sdk/x/feegrant/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	upgradekeeper "github.com/cosmos/cosmos-sdk/x/upgrade/keeper"
	ibctransferkeeper "github.com/cosmos/ibc-go/v4/modules/apps/transfer/keeper"
	ibckeeper "github.com/cosmos/ibc-go/v4/modules/core/keeper"

	akeeper "github.com/akash-network/node/x/audit/keeper"
	ckeeper "github.com/akash-network/node/x/cert/keeper"
	dkeeper "github.com/akash-network/node/x/deployment/keeper"
	escrowkeeper "github.com/akash-network/node/x/escrow/keeper"
	agovkeeper "github.com/akash-network/node/x/gov/keeper"
	ikeeper "github.com/akash-network/node/x/inflation/keeper"
	mkeeper "github.com/akash-network/node/x/market/keeper"
	pkeeper "github.com/akash-network/node/x/provider/keeper"
	astakingkeeper "github.com/akash-network/node/x/staking/keeper"
	tkeeper "github.com/akash-network/node/x/take/keeper"
)

var ErrEmptyFieldName = errors.New("empty field name")

type AppKeepers struct {
	Cosmos struct {
		Acct                 authkeeper.AccountKeeper
		Authz                authzkeeper.Keeper
		FeeGrant             feegrantkeeper.Keeper
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
		Take       tkeeper.IKeeper
		Market     mkeeper.IKeeper
		Provider   pkeeper.IKeeper
		Audit      akeeper.Keeper
		Cert       ckeeper.Keeper
		Inflation  ikeeper.IKeeper
		Staking    astakingkeeper.IKeeper
		Gov        agovkeeper.IKeeper
	}
}

type App struct {
	Keepers      AppKeepers
	Configurator module.Configurator
	MM           *module.Manager
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
