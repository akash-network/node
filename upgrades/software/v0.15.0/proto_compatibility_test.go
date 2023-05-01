package v0_15_0_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	dv1beta1 "github.com/akash-network/akash-api/go/node/deployment/v1beta1"
	dv1beta2 "github.com/akash-network/akash-api/go/node/deployment/v1beta2"
	ev1beta1 "github.com/akash-network/akash-api/go/node/escrow/v1beta1"
	ev1beta2 "github.com/akash-network/akash-api/go/node/escrow/v1beta2"
	mv1beta1 "github.com/akash-network/akash-api/go/node/market/v1beta1"
	mv1beta2 "github.com/akash-network/akash-api/go/node/market/v1beta2"
	types "github.com/akash-network/akash-api/go/node/types/v1beta1"

	"github.com/akash-network/node/app"
)

var (
	cdc = app.MakeEncodingConfig().Marshaler.(codec.BinaryCodec)
)

func TestDeployment_DeploymentProto_IsCompatible(t *testing.T) {
	oldProto := dv1beta1.Deployment{
		DeploymentID: dv1beta1.DeploymentID{Owner: "A", DSeq: 1},
		State:        dv1beta1.DeploymentActive,
		Version:      []byte{1, 2, 3, 4},
		CreatedAt:    5,
	}

	expectedProto := dv1beta2.Deployment{
		DeploymentID: dv1beta2.DeploymentID{Owner: "A", DSeq: 1},
		State:        dv1beta2.DeploymentActive,
		Version:      []byte{1, 2, 3, 4},
		CreatedAt:    5,
	}

	var actualProto dv1beta2.Deployment
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)
	require.Equal(t, expectedProto, actualProto)
	require.Equal(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto))
}

func TestDeployment_GroupProto_IsNotCompatible(t *testing.T) {
	oldProto := dv1beta1.Group{
		GroupID: dv1beta1.GroupID{
			Owner: "A",
			DSeq:  1,
			GSeq:  2,
		},
		State: dv1beta1.GroupOpen,
		GroupSpec: dv1beta1.GroupSpec{
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
			Resources: []dv1beta1.Resource{{
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

	var actualProto dv1beta2.Group
	require.Error(t, cdc.Unmarshal(cdc.MustMarshal(&oldProto), &actualProto)) // it doesn't unmarshal
}

func TestEscrow_AccountProto_IsNotCompatible(t *testing.T) {
	oldProto := ev1beta1.Account{
		ID: ev1beta1.AccountID{
			Scope: "a",
			XID:   "a",
		},
		Owner:       "a",
		State:       1,
		Balance:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		Transferred: sdk.NewCoin("uakt", sdk.NewInt(1)),
		SettledAt:   2,
	}

	expectedProto := ev1beta2.Account{
		ID: ev1beta2.AccountID{
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

	var actualProto ev1beta2.Account
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestEscrow_PaymentProto_IsNotCompatible(t *testing.T) {
	oldProto := ev1beta1.Payment{
		AccountID: ev1beta1.AccountID{
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

	expectedProto := ev1beta2.FractionalPayment{
		AccountID: ev1beta2.AccountID{
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

	var actualProto ev1beta2.FractionalPayment
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_BidProto_IsNotCompatible(t *testing.T) {
	oldProto := mv1beta1.Bid{
		BidID: mv1beta1.BidID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mv1beta1.BidActive,
		Price:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	expectedProto := mv1beta2.Bid{
		BidID: mv1beta2.BidID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mv1beta2.BidActive,
		Price:     sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	var actualProto mv1beta2.Bid
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_LeaseProto_IsNotCompatible(t *testing.T) {
	oldProto := mv1beta1.Lease{
		LeaseID: mv1beta1.LeaseID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mv1beta1.LeaseActive,
		Price:     sdk.NewCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	expectedProto := mv1beta2.Lease{
		LeaseID: mv1beta2.LeaseID{
			Owner:    "a",
			DSeq:     1,
			GSeq:     2,
			OSeq:     3,
			Provider: "a",
		},
		State:     mv1beta2.LeaseActive,
		Price:     sdk.NewDecCoin("uakt", sdk.NewInt(1)),
		CreatedAt: 1,
	}

	var actualProto mv1beta2.Lease
	cdc.MustUnmarshal(cdc.MustMarshal(&oldProto), &actualProto)                      // although it unmarshalls
	require.NotEqual(t, expectedProto, actualProto)                                  // but the result isn't equal
	require.NotEqual(t, cdc.MustMarshal(&oldProto), cdc.MustMarshal(&expectedProto)) // neither is marshalled bytes
}

func TestMarket_OrderProto_IsNotCompatible(t *testing.T) {
	oldProto := mv1beta1.Order{
		OrderID: mv1beta1.OrderID{
			Owner: "a",
			DSeq:  1,
			GSeq:  2,
			OSeq:  3,
		},
		State: mv1beta1.OrderActive,
		Spec: dv1beta1.GroupSpec{
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
			Resources: []dv1beta1.Resource{{
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

	var actualProto mv1beta2.Order
	require.Error(t, cdc.Unmarshal(cdc.MustMarshal(&oldProto), &actualProto)) // it doesn't unmarshal
}
