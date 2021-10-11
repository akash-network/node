package v1beta1

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	MsgTypeCreateCertificate = "cert-create-certificate"
	MsgTypeRevokeCertificate = "cert-revoke-certificate"
)

var (
	_ sdk.Msg = &MsgCreateCertificate{}
	_ sdk.Msg = &MsgRevokeCertificate{}
)

// ====MsgCreateCertificate====
// Route implements the sdk.Msg interface
func (m MsgCreateCertificate) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgCreateCertificate) Type() string {
	return MsgTypeCreateCertificate
}

// ValidateBasic does basic validation
func (m MsgCreateCertificate) ValidateBasic() error {
	owner, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgCreate: Invalid Owner Address")
	}

	_, err = ParseAndValidateCertificate(owner, m.Cert, m.Pubkey)
	if err != nil {
		return err
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgCreateCertificate) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgCreateCertificate) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}

// ====MsgRevokeCertificate====
// Route implements the sdk.Msg interface
func (m MsgRevokeCertificate) Route() string {
	return RouterKey
}

// Type implements the sdk.Msg interface
func (m MsgRevokeCertificate) Type() string {
	return MsgTypeRevokeCertificate
}

// ValidateBasic does basic validation
func (m MsgRevokeCertificate) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.ID.Owner); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "MsgRevoke: Invalid Owner Address")
	}

	if _, valid := new(big.Int).SetString(m.ID.Serial, 10); !valid {
		return ErrInvalidSerialNumber
	}

	return nil
}

// GetSignBytes encodes the message for signing
func (m MsgRevokeCertificate) GetSignBytes() []byte {
	return sdk.MustSortJSON(ModuleCdc.MustMarshalJSON(&m))
}

// GetSigners defines whose signature is required
func (m MsgRevokeCertificate) GetSigners() []sdk.AccAddress {
	owner, err := sdk.AccAddressFromBech32(m.ID.Owner)
	if err != nil {
		panic(err)
	}

	return []sdk.AccAddress{owner}
}
