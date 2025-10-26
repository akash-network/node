package cert

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/node/x/cert/keeper"

	types "pkg.akt.dev/go/node/cert/v1"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Certificates {
		if err := record.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.Keeper, data *types.GenesisState) {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Certificates {
		owner, err := sdk.AccAddressFromBech32(record.Owner)
		if err != nil {
			panic(fmt.Sprintf("error init certificate from genesis: %s", err))
		}

		cert, err := types.ParseAndValidateCertificate(owner, record.Certificate.Cert, record.Certificate.Pubkey)
		if err != nil {
			panic(err.Error())
		}

		key := keeper.MustCertificateKey(record.Certificate.State, types.CertID{
			Owner:  owner,
			Serial: *cert.SerialNumber,
		})

		if store.Has(key) {
			panic(types.ErrCertificateExists.Error())
		}

		store.Set(key, cdc.MustMarshal(&record.Certificate))
	}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	var res types.GenesisCertificates

	k.WithCertificates(ctx, func(id types.CertID, certificate types.CertificateResponse) bool {
		block, rest := pem.Decode(certificate.Certificate.Cert)
		if len(rest) > 0 {
			panic("unable to decode certificate")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			panic(err.Error())
		}

		if cert.SerialNumber.String() != id.Serial.String() {
			panic("certificate id does not match")
		}

		res = append(res, types.GenesisCertificate{
			Owner:       id.Owner.String(),
			Certificate: certificate.Certificate,
		})

		return false
	})

	return &types.GenesisState{
		Certificates: res,
	}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

// GetGenesisStateFromAppState returns x/cert GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
