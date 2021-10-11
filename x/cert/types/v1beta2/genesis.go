package v1beta2

import (
	"bytes"
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type GenesisCertificates []GenesisCertificate

func (obj GenesisCertificates) Contains(cert GenesisCertificate) bool {
	for _, c := range obj {
		if c.Owner == cert.Owner {
			return true
		}

		if bytes.Equal(c.Certificate.Cert, cert.Certificate.Cert) {
			return true
		}
	}

	return false
}

func (m GenesisCertificate) Validate() error {
	owner, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		return err
	}
	if err := m.Certificate.Validate(owner); err != nil {
		return err
	}

	return nil
}

func (m *GenesisState) Validate() error {
	for _, cert := range m.Certificates {
		if err := cert.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// GetGenesisStateFromAppState returns x/cert GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *GenesisState {
	var genesisState GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}
