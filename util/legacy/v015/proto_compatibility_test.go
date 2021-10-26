package v015_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/app"
	types "github.com/ovrclk/akash/types/v1beta1"
	dtypesv1beta1 "github.com/ovrclk/akash/x/deployment/types/v1beta1"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	etypesv1beta1 "github.com/ovrclk/akash/x/escrow/types/v1beta1"
	etypes "github.com/ovrclk/akash/x/escrow/types/v1beta2"
	mtypesv1beta1 "github.com/ovrclk/akash/x/market/types/v1beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/stretchr/testify/require"
)

var (
	cdc = app.MakeEncodingConfig().Marshaler.(codec.BinaryCodec)
)

func TestDeployment_DeploymentProto_IsCompatible(t *testing.T) {
	oldProto := dtypesv1beta1.Deployment{
		DeploymentID: dtypesv1beta1.DeploymentID{Owner: "A", DSeq: 1},
		State:        dtypesv1beta1.DeploymentActive,
		Version:      []byte{1, 2, 3, 4},
		CreatedAt:    5,
	}

	expectedProto := dtypes.Deployment{
		DeploymentID: dtypes.DeploymentID{Owner: "A", DSeq: 1},
		State:        dtypes.DeploymentActive,
		Version:      []byte{1, 2, 3, 4},
		CreatedAt:    5,
	}

	var actualProto dtypes.Deployment
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)
	require.Equal(t, expectedProto, actualProto)
	require.Equal(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto))
}

func TestDeployment_GroupProto_IsNotCompatible(t *testing.T) {
	oldProto := dtypesv1beta1.Group{
		GroupID: dtypesv1beta1.GroupID{
			Owner: "A",
			DSeq:  1,
			GSeq:  2,
		},
		State: dtypesv1beta1.GroupOpen,
		GroupSpec: dtypesv1beta1.GroupSpec{
			Name: "A",
			Requirements: types.PlacementRequirements{
				SignedBy: types.SignedBy{
					AllOf: []string{"a"},
					AnyOf: []string{"a"},
				},
				Attributes: types.Attributes{{
					Key:   "a",
					Value: "a",
				}},
			},
			Resources: []dtypesv1beta1.Resource{{
				Resources: types.ResourceUnits{
					CPU: &types.CPU{
						Units: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Memory: &types.Memory{
						Quantity: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Storage: &types.Storage{
						Quantity: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Endpoints: []types.Endpoint{{Kind: types.Endpoint_RANDOM_PORT}},
				},
				Count: 1,
				Price: sdk.NewCoin("uakt", sdk.NewInt(1)),
			}},
		},
		CreatedAt: 5,
	}

	var actualProto dtypes.Group
	require.Error(t, cdc.Unmarshal(cdc.MustMarshal(&oldProto), &actualProto)) // it doesn't unmarshal
}

func TestEscrow_AccountProto_IsNotCompatible(t *testing.T) {
	oldProto := etypesv1beta1.Account{
		ID: etypesv1beta1.AccountID{
			Scope: "a",
			XID:   "a",
		},
		Owner:       "a",
		State:       1,
		Balance:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		Transferred: sdk.NewCoin("uakt", sdk.NewInt(1)),
		SettledAt:   2,
	}

	expectedProto := etypes.Account{
		ID: etypes.AccountID{
			Scope: "a",
			XID:   "a",
		},
		Owner:       "a",
		State:       1,
		Balance:     sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		Transferred: sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		SettledAt:   2,
		Depositor:   "",
		Funds:       sdk.DecCoin{},
	}

	var actualProto etypes.Account
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestEscrow_PaymentProto_IsNotCompatible(t *testing.T) {
	oldProto := etypesv1beta1.Payment{
		AccountID: etypesv1beta1.AccountID{
			Scope: "a",
			XID:   "a",
		},
		PaymentID: "a",
		Owner:     "a",
		State:     1,
		Rate:      sdk.NewCoin("uakt", sdk.NewInt(1)),
		Balance:   sdk.NewCoin("uakt", sdk.NewInt(1)),
		Withdrawn: sdk.NewCoin("uakt", sdk.NewInt(1)),
	}

	expectedProto := etypes.FractionalPayment{
		AccountID: etypes.AccountID{
			Scope: "a",
			XID:   "a",
		},
		PaymentID: "a",
		Owner:     "a",
		State:     1,
		Rate:      sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		Balance:   sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		Withdrawn: sdk.NewCoin("uakt", sdk.NewInt(1)),
	}

	var actualProto etypes.FractionalPayment
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_BidProto_IsNotCompatible(t *testing.T) {
	oldProto := mtypesv1beta1.Bid{
		BidID: mtypesv1beta1.BidID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mtypesv1beta1.BidActive,
		Price:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	expectedProto := mtypes.Bid{
		BidID: mtypes.BidID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mtypes.BidActive,
		Price:     sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	var actualProto mtypes.Bid
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_LeaseProto_IsNotCompatible(t *testing.T) {
	oldProto := mtypesv1beta1.Lease{
		LeaseID: mtypesv1beta1.LeaseID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mtypesv1beta1.LeaseActive,
		Price:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	expectedProto := mtypes.Lease{
		LeaseID: mtypes.LeaseID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mtypes.LeaseActive,
		Price:     sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	var actualProto mtypes.Lease
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_OrderProto_IsNotCompatible(t *testing.T) {
	oldProto := mtypesv1beta1.Order{
		OrderID: mtypesv1beta1.OrderID{
			Owner: "a",
			DSeq:  1,
			GSeq:  2,
			OSeq:  3,
		},
		State: mtypesv1beta1.OrderActive,
		Spec: dtypesv1beta1.GroupSpec{
			Name: "A",
			Requirements: types.PlacementRequirements{
				SignedBy: types.SignedBy{
					AllOf: []string{"a"},
					AnyOf: []string{"a"},
				},
				Attributes: types.Attributes{{
					Key:   "a",
					Value: "a",
				}},
			},
			Resources: []dtypesv1beta1.Resource{{
				Resources: types.ResourceUnits{
					CPU: &types.CPU{
						Units: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Memory: &types.Memory{
						Quantity: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Storage: &types.Storage{
						Quantity: types.ResourceValue{Val: sdk.NewInt(1)},
						Attributes: types.Attributes{{
							Key:   "a",
							Value: "a",
						}},
					},
					Endpoints: []types.Endpoint{{Kind: types.Endpoint_RANDOM_PORT}},
				},
				Count: 1,
				Price: sdk.NewCoin("uakt", sdk.NewInt(1)),
			}},
		},
		CreatedAt: 1,
	}

	var actualProto mtypes.Order
	require.Error(t, cdc.Unmarshal(cdc.MustMarshal(&oldProto), &actualProto)) // it doesn't unmarshal
}
