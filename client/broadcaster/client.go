package broadcaster

import (
	"context"
	"errors"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

var (
	// ErrBroadcastTx is used when a broadcast fails due to tendermint errors
	ErrBroadcastTx = errors.New("broadcast tx error")
	// ErrSimulateTx is used when unsigned tx is not proto tx provider format
	ErrSimulateTx = errors.New("cannot simulate amino tx")
)

type Client interface {
	Broadcast(ctx context.Context, msgs ...sdk.Msg) error
}

type simpleClient struct {
	cctx sdkclient.Context
	txf  tx.Factory
	info keyring.Info
}

func NewClient(cctx sdkclient.Context, txf tx.Factory, info keyring.Info) Client {
	return &simpleClient{
		cctx: cctx,
		txf:  txf,
		info: info,
	}
}

func (c *simpleClient) Broadcast(_ context.Context, msgs ...sdk.Msg) error {
	txf, err := PrepareFactory(c.cctx, c.txf)
	if err != nil {
		return err
	}

	response, err := doBroadcast(c.cctx, txf, c.info.GetName(), msgs...)
	if err != nil {
		return err
	}

	if response.Code != 0 {
		return fmt.Errorf("%w: response code %d - (%#v)", ErrBroadcastTx, response.Code, response)
	}
	return nil
}

func doBroadcast(cctx sdkclient.Context, txf tx.Factory, keyName string, msgs ...sdk.Msg) (*sdk.TxResponse, error) {
	txn, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return nil, err
	}

	err = tx.Sign(txf, keyName, txn, true)
	if err != nil {
		return nil, err
	}

	bytes, err := cctx.TxConfig.TxEncoder()(txn.GetTx())
	if err != nil {
		return nil, err
	}

	response, err := cctx.BroadcastTxSync(bytes)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// PrepareFactory ensures the account defined by ctx.GetFromAddress() exists and
// if the account number and/or the account sequence number are zero (not set),
// they will be queried for and set on the provided Factory. A new Factory with
// the updated fields will be returned.
func PrepareFactory(clientCtx sdkclient.Context, txf tx.Factory) (tx.Factory, error) {
	from := clientCtx.GetFromAddress()

	if err := txf.AccountRetriever().EnsureExists(clientCtx, from); err != nil {
		return txf, err
	}

	initNum, initSeq := txf.AccountNumber(), txf.Sequence()
	if initNum == 0 || initSeq == 0 {
		num, seq, err := txf.AccountRetriever().GetAccountNumberSequence(clientCtx, from)
		if err != nil {
			return txf, err
		}

		if initNum == 0 {
			txf = txf.WithAccountNumber(num)
		}

		if initSeq == 0 {
			txf = txf.WithSequence(seq)
		}
	}

	return txf, nil
}

// CalculateGas simulates the execution of a transaction and returns the
// simulation response obtained by the query and the adjusted gas amount.
func CalculateGas(
	queryFunc func(string, []byte) ([]byte, int64, error), txf tx.Factory, msgs ...sdk.Msg,
) (txtypes.SimulateResponse, uint64, error) {
	txBytes, err := BuildSimTx(txf, msgs...)
	if err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	bz, _, err := queryFunc("/cosmos.tx.v1beta1.Service/Simulate", txBytes)
	if err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	var simRes txtypes.SimulateResponse

	if err := simRes.Unmarshal(bz); err != nil {
		return txtypes.SimulateResponse{}, 0, err
	}

	return simRes, uint64(txf.GasAdjustment() * float64(simRes.GasInfo.GasUsed)), nil
}

// protoTxProvider is a type which can provide a proto transaction. It is a
// workaround to get access to the wrapper TxBuilder's method GetProtoTx().
type protoTxProvider interface {
	GetProtoTx() *txtypes.Tx
}

// BuildSimTx creates an unsigned tx with an empty single signature and returns
// the encoded transaction or an error if the unsigned transaction cannot be
// built.
func BuildSimTx(txf tx.Factory, msgs ...sdk.Msg) ([]byte, error) {
	txb, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return nil, err
	}

	// Create an empty signature literal as the ante handler will populate with a
	// sentinel pubkey.
	sig := signing.SignatureV2{
		PubKey: &secp256k1.PubKey{},
		Data: &signing.SingleSignatureData{
			SignMode: txf.SignMode(),
		},
		Sequence: txf.Sequence(),
	}
	if err := txb.SetSignatures(sig); err != nil {
		return nil, err
	}

	protoProvider, ok := txb.(protoTxProvider)
	if !ok {
		return nil, fmt.Errorf("%w: not protoTxProvider format", ErrSimulateTx)
	}
	simReq := txtypes.SimulateRequest{Tx: protoProvider.GetProtoTx()}

	return simReq.Marshal()
}
