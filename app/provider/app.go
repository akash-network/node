package provider

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/types/code"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	Name = apptypes.TagAppProvider
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.ProviderPath)
}

func (a *app) Query(state appstate.State, req abci_types.RequestQuery) abci_types.ResponseQuery {

	if !a.AcceptQuery(req) {
		return abci_types.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}

	// TODO: Partial Key Parsing
	id := strings.TrimPrefix(req.Path, appstate.ProviderPath)
	if len(id) == 0 {
		return a.doRangeQuery(state)
	}

	key, err := keys.ParseProviderPath(id)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	return a.doQuery(state, key.ID())
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateProvider:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateProvider:
		return a.doCheckTx(state, ctx, tx.TxCreateProvider)
	}
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateProvider:
		return a.doDeliverTx(state, ctx, tx.TxCreateProvider)
	}
	return abci_types.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) doQuery(state appstate.State, key base.Bytes) abci_types.ResponseQuery {

	provider, err := state.Provider().Get(key)

	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if provider == nil {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("provider %x not found", key),
		}
	}

	bytes, err := proto.Marshal(provider)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doRangeQuery(state appstate.State) abci_types.ResponseQuery {
	dcs, err := state.Provider().GetMaxRange()
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if len(dcs.Providers) == 0 {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("providers not found"),
		}
	}

	bytes, err := proto.Marshal(dcs)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doCheckTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateProvider) abci_types.ResponseCheckTx {
	if !bytes.Equal(ctx.Signer().Address(), tx.Owner) {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by owner",
		}
	}

	signer, err_ := state.Account().Get(tx.Owner)
	if err_ != nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "unknown source account",
		}
	}

	if signer == nil && tx.Nonce != 1 {
		return abci_types.ResponseCheckTx{Code: code.INVALID_TRANSACTION, Log: "invalid nonce"}
	} else if signer != nil && signer.Nonce >= tx.Nonce {
		return abci_types.ResponseCheckTx{Code: code.INVALID_TRANSACTION, Log: "invalid nonce"}
	}

	if _, err := url.Parse(tx.HostURI); err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "invalid network address",
		}
	}

	return abci_types.ResponseCheckTx{}
}

func (a *app) doDeliverTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateProvider) abci_types.ResponseDeliverTx {

	cresp := a.doCheckTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	provider := &types.Provider{
		Address:    appstate.ProviderAddress(tx.Owner, tx.Nonce),
		Owner:      tx.Owner,
		HostURI:    tx.HostURI,
		Attributes: tx.Attributes,
	}

	if err := state.Provider().Save(provider); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeProviderCreate),
		Data: provider.Address,
	}
}
