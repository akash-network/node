package types

import (
	"fmt"
	"net/url"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/types"
)

func TestConfigPath(t *testing.T) {
	type testConfigPath struct {
		path   string
		expErr error
	}
	tests := []testConfigPath{
		{
			path:   "/home/ropes/go/src/github.com/ovrclk/akash/_run/kube/provider.yaml",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "foo.yaml",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "localhost",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "localhost/foo",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "localhost:80",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "localhost:80/foo",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "127.0.0.1",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "127.0.0.1/foo",
			expErr: ErrNotAbsProviderURI,
		},
		{
			path:   "127.0.0.1:80",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "127.0.0.1:80/foo",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "file:///foo.yaml",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "http://localhost",
			expErr: nil,
		},
		{
			path:   "http://localhost/foo",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "http://localhost:80",
			expErr: nil,
		},
		{
			path:   "http://localhost:80/foo",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "http://localhost:3001/",
			expErr: ErrInvalidProviderURI,
		},
		{
			path:   "https://localhost:80",
			expErr: nil,
		},
		{
			path:   "https://localhost:80/foo",
			expErr: ErrInvalidProviderURI,
		},
	}

	for i, testUnit := range tests {
		closure := func(test testConfigPath) func(t *testing.T) {
			testFunc := func(t *testing.T) {
				err := validateProviderURI(test.path)
				if test.expErr != nil && !errors.Is(err, test.expErr) ||
					err != nil && test.expErr == nil {
					t.Errorf("unexpected error occurred: %v", err)

					_, err := url.Parse(test.path)
					if err != nil {
						t.Errorf("url.Parse() of %q err: %v", test.path, err)
					}
				}
			}
			return testFunc
		}
		tf := closure(testUnit)
		t.Run(fmt.Sprintf("%d->%q", i, testUnit.path), tf)
	}
}

type providerTestParams struct {
	msg    Provider
	expErr error
	delErr error
}

func (test providerTestParams) testCreate() func(t *testing.T) {
	msg := MsgCreateProvider{
		Owner:      test.msg.Owner,
		HostURI:    test.msg.HostURI,
		Attributes: test.msg.Attributes,
	}
	vErr := msg.ValidateBasic()
	return func(t *testing.T) {
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
	}
}

func (test providerTestParams) testUpdate() func(t *testing.T) {
	msg := MsgUpdateProvider{
		Owner:      test.msg.Owner,
		HostURI:    test.msg.HostURI,
		Attributes: test.msg.Attributes,
	}
	vErr := msg.ValidateBasic()
	return func(t *testing.T) {
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
	}
}

func (test providerTestParams) testDelete() func(t *testing.T) {
	msg := MsgDeleteProvider{
		Owner: test.msg.Owner,
	}
	vErr := msg.ValidateBasic()
	return func(t *testing.T) {
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
	}
}

var msgCreateTests = []providerTestParams{
	{
		msg: Provider{
			Owner:   sdk.AccAddress("hihi"),
			HostURI: "http://localhost:3001",
			Attributes: []types.Attribute{
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
			HostURI: "http://localhost:3001",
			Attributes: []types.Attribute{
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
			Attributes: []types.Attribute{
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
			Attributes: []types.Attribute{
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
		main := func(test providerTestParams) func(t *testing.T) {
			return func(t *testing.T) {
				t.Run("msg-create", test.testCreate())
				t.Run("msg-update", test.testUpdate())
				t.Run("msg-delete", test.testDelete())
			}
		}
		f := main(test)
		t.Run(fmt.Sprintf("%d", i), f)
	}
}
