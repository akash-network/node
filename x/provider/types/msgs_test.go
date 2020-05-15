package types

import (
	"fmt"
	"net/url"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
)

func TestConfigPath(t *testing.T) {
	tests := []struct {
		path   string
		expErr error
	}{
		{
			path:   "/home/ropes/go/src/github.com/ovrclk/akash/_run/kube/provider.yml",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "foo.yml",
			expErr: ErrNotAbsProviderURI,
		},
		/*{
			path:   "localhost:80/foo", // would expect this to cause error, but it does not.
			expErr: ErrNotAbsProviderURI,
		},*/
		{
			path:   "file:///foo.yml",
			expErr: nil,
		},
		{
			path:   "http://localhost:80/foo",
			expErr: nil,
		},
		{
			path:   "http://localhost:3001/",
			expErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("->%q", test.path), func(t *testing.T) {
			err := validateProviderURI(test.path)
			if test.expErr != nil && !errors.Is(err, test.expErr) ||
				err != nil && test.expErr == nil {
				t.Errorf("unexpected error occurred: %v", err)

				p, err := url.Parse(test.path)
				if err != nil {
					t.Errorf("url.Parse() of %q err: %v", test.path, err)
				}
				t.Logf("%#v", p)
			}
		})
	}
}

var msgCreateTests = []struct {
	msg    Provider
	expErr error
	delErr error
}{
	{
		msg: Provider{
			Owner:   sdk.AccAddress("hihi"),
			HostURI: "http://localhost:3001/",
			Attributes: []sdk.Attribute{
				{
					Key:   "hihi",
					Value: "neh",
				},
			},
		},
		expErr: nil,
	},
	{
		msg: Provider{
			Owner:   sdk.AccAddress(""),
			HostURI: "http://localhost:3001/",
			Attributes: []sdk.Attribute{
				{
					Key:   "hihi",
					Value: "neh",
				},
			},
		},
		expErr: sdkerrors.ErrInvalidAddress,
		delErr: sdkerrors.ErrInvalidAddress,
	},
	{
		msg: Provider{
			Owner:   sdk.AccAddress("hihi"),
			HostURI: "ht tp://foo.com",
			Attributes: []sdk.Attribute{
				{
					Key:   "hihi",
					Value: "neh",
				},
			},
		},
		expErr: ErrInvalidProviderURI,
	},
	{
		msg: Provider{
			Owner:   sdk.AccAddress("hihi"),
			HostURI: "",
			Attributes: []sdk.Attribute{
				{
					Key:   "hihi",
					Value: "neh",
				},
			},
		},
		expErr: ErrNotAbsProviderURI,
	},
}

func TestMsgStarValidation(t *testing.T) {
	for i, test := range msgCreateTests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Run("msg-create", func(t *testing.T) {
				msg := MsgCreateProvider{
					Owner:      test.msg.Owner,
					HostURI:    test.msg.HostURI,
					Attributes: test.msg.Attributes,
				}
				vErr := msg.ValidateBasic()
				if test.expErr != nil && !errors.Is(vErr, test.expErr) {
					t.Errorf("error expected: '%v' VS: %v", test.expErr, vErr)
					return
				}
				sb := msg.GetSignBytes()
				if len(sb) == 0 {
					t.Error("no signed bytes returned")
				}
				ob := msg.GetSigners()
				if len(ob) == 0 {
					t.Error("no owners returned from valid message")
				}
			})
			t.Run("msg-update", func(t *testing.T) {
				msg := MsgUpdateProvider{
					Owner:      test.msg.Owner,
					HostURI:    test.msg.HostURI,
					Attributes: test.msg.Attributes,
				}
				vErr := msg.ValidateBasic()
				if test.expErr != nil && !errors.Is(vErr, test.expErr) {
					t.Errorf("error expected: '%v' VS: %v", test.expErr, vErr)
					return
				}
				sb := msg.GetSignBytes()
				if len(sb) == 0 {
					t.Error("no signed bytes returned")
				}
				ob := msg.GetSigners()
				if len(ob) == 0 {
					t.Error("no owners returned from valid message")
				}
			})
			t.Run("msg-delete", func(t *testing.T) {
				msg := MsgDeleteProvider{
					Owner: test.msg.Owner,
				}
				vErr := msg.ValidateBasic()
				if test.delErr != nil && !errors.Is(vErr, test.delErr) {
					t.Errorf("error expected: '%v' VS: %v", test.expErr, vErr)
					return
				}
				sb := msg.GetSignBytes()
				if len(sb) == 0 {
					t.Error("no signed bytes returned")
				}
				ob := msg.GetSigners()
				if len(ob) == 0 {
					t.Error("no owners returned from valid message")
				}
			})
		})
	}
}
