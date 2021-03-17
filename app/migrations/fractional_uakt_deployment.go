package migrations

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	akashtypes "github.com/ovrclk/akash/types"
	oldakashtypes "github.com/ovrclk/akash/typesv1beta1"
	newAudit "github.com/ovrclk/akash/x/audit/types"
	oldAudit "github.com/ovrclk/akash/x/audit/typesv1beta1"
	newCert "github.com/ovrclk/akash/x/cert/types"
	oldCert "github.com/ovrclk/akash/x/cert/typesv1beta1"
	newDeployment "github.com/ovrclk/akash/x/deployment/types"
	oldDeployment "github.com/ovrclk/akash/x/deployment/typesv1beta1"
	newEscrow "github.com/ovrclk/akash/x/escrow/types"
	oldEscrow "github.com/ovrclk/akash/x/escrow/typesv1beta1"
	newProvider "github.com/ovrclk/akash/x/provider/types"
	oldProvider "github.com/ovrclk/akash/x/provider/typesv1beta1"

	newMarket "github.com/ovrclk/akash/x/market/types"
	oldMarket "github.com/ovrclk/akash/x/market/typesv1beta1"
)

func fractionalUaktMigrateGroup(groupRaw []byte) (newDeployment.Group, error) {
	var oldObject oldDeployment.Group
	err := oldObject.Unmarshal(groupRaw)
	if err != nil {
		return newDeployment.Group{}, err
	}

	newObject := newDeployment.Group{
		GroupID: newDeployment.GroupID{
			Owner: oldObject.GroupID.Owner,
			DSeq:  oldObject.GroupID.DSeq,
			GSeq:  oldObject.GroupID.GSeq,
		},
		State: newDeployment.Group_State(oldObject.State),
		GroupSpec: newDeployment.GroupSpec{
			Name:         oldObject.GroupSpec.Name,
			Requirements: fractionalUaktMigratePlacementRequirements(oldObject.GroupSpec.Requirements),
			Resources:    fractionalUaktMigrateResources(oldObject.GroupSpec.Resources),
		},
		CreatedAt: oldObject.CreatedAt,
	}

	return newObject, nil
}

func fractionalUaktMigrateDeployment(deploymentRaw []byte) (newDeployment.Deployment, error) {
	var oldObject oldDeployment.Deployment
	err := oldObject.Unmarshal(deploymentRaw)
	if err != nil {
		return newDeployment.Deployment{}, err
	}

	newObject := newDeployment.Deployment{
		DeploymentID: newDeployment.DeploymentID{
			Owner: oldObject.DeploymentID.Owner,
			DSeq:  oldObject.DeploymentID.DSeq,
		},
		State:     newDeployment.Deployment_State(oldObject.State),
		Version:   oldObject.Version,
		CreatedAt: oldObject.CreatedAt,
	}

	return newObject, nil
}

func fractionalUaktMigrateLease(leaseRaw []byte) (newMarket.Lease, error) {
	var oldObject oldMarket.Lease
	err := oldObject.Unmarshal(leaseRaw)
	if err != nil {
		return newMarket.Lease{}, err
	}

	newObject := newMarket.Lease{
		LeaseID: newMarket.LeaseID{
			Owner:    oldObject.LeaseID.Owner,
			DSeq:     oldObject.LeaseID.DSeq,
			GSeq:     oldObject.LeaseID.GSeq,
			OSeq:     oldObject.LeaseID.OSeq,
			Provider: oldObject.LeaseID.Provider,
		},
		State: newMarket.Lease_State(oldObject.State),
		Price: sdk.DecCoin{
			Denom:  oldObject.Price.Denom,
			Amount: sdk.NewDecFromInt(oldObject.Price.Amount),
		},
		CreatedAt: oldObject.CreatedAt,
	}

	return newObject, nil
}

func fractionalUaktMigrateBid(bidRaw []byte) (newMarket.Bid, error) {
	var oldObject oldMarket.Bid
	err := oldObject.Unmarshal(bidRaw)
	if err != nil {
		return newMarket.Bid{}, err
	}

	newObject := newMarket.Bid{
		BidID: newMarket.BidID{
			Owner:    oldObject.BidID.Owner,
			DSeq:     oldObject.BidID.DSeq,
			GSeq:     oldObject.BidID.GSeq,
			OSeq:     oldObject.BidID.OSeq,
			Provider: oldObject.BidID.Provider,
		},
		State: newMarket.Bid_State(oldObject.State),
		Price: sdk.DecCoin{
			Denom:  oldObject.Price.Denom,
			Amount: sdk.NewDecFromInt(oldObject.Price.Amount),
		},
		CreatedAt: oldObject.CreatedAt,
	}

	return newObject, nil
}

func fractionalUaktMigratePlacementRequirements(r oldakashtypes.PlacementRequirements) akashtypes.PlacementRequirements {
	attrs := make([]akashtypes.Attribute, len(r.Attributes))

	for i, v := range r.Attributes {
		attrs[i] = akashtypes.Attribute{
			Key:   v.Key,
			Value: v.Value,
		}
	}

	result := akashtypes.PlacementRequirements{
		SignedBy: akashtypes.SignedBy{
			AllOf: r.SignedBy.AllOf,
			AnyOf: r.SignedBy.AnyOf,
		},
		Attributes: attrs,
	}

	return result
}

func fractionalUaktMigrateAttributes(oldAttrs []oldakashtypes.Attribute) []akashtypes.Attribute {
	result := make([]akashtypes.Attribute, len(oldAttrs))
	for i, oldAttr := range oldAttrs {
		result[i] = akashtypes.Attribute{
			Key:   oldAttr.Key,
			Value: oldAttr.Value,
		}
	}

	return result
}

func fractionalUaktMigrateResources(r []oldDeployment.Resource) []newDeployment.Resource {
	result := make([]newDeployment.Resource, len(r))

	for i, v := range r {

		entry := newDeployment.Resource{
			Resources: akashtypes.ResourceUnits{
				CPU: &akashtypes.CPU{
					Units: akashtypes.ResourceValue{
						Val: v.Resources.CPU.Units.Val,
					},
					Attributes: fractionalUaktMigrateAttributes(v.Resources.CPU.Attributes),
				},
				Memory: &akashtypes.Memory{
					Quantity: akashtypes.ResourceValue{
						Val: v.Resources.Memory.Quantity.Val,
					},
					Attributes: fractionalUaktMigrateAttributes(v.Resources.Memory.Attributes),
				},
				Storage: &akashtypes.Storage{
					Quantity: akashtypes.ResourceValue{
						Val: v.Resources.Storage.Quantity.Val,
					},
					Attributes: fractionalUaktMigrateAttributes(v.Resources.Storage.Attributes),
				},
				Endpoints: make([]akashtypes.Endpoint, len(v.Resources.Endpoints)),
			},
			Count: v.Count,
			Price: sdk.DecCoin{
				Denom:  v.Price.Denom,
				Amount: sdk.NewDecFromInt(v.Price.Amount),
			},
		}
		result[i] = entry
	}

	return result
}

func fractionalUaktMigrateOrder(orderRaw []byte) (newMarket.Order, error) {
	var oldObject oldMarket.Order
	err := oldObject.Unmarshal(orderRaw)
	if err != nil {
		return newMarket.Order{}, err
	}

	newPlacementRequirements := fractionalUaktMigratePlacementRequirements(oldObject.Spec.Requirements)
	newResources := fractionalUaktMigrateResources(oldObject.Spec.Resources)

	newObject := newMarket.Order{
		OrderID: newMarket.OrderID{
			Owner: oldObject.OrderID.Owner,
			DSeq:  oldObject.OrderID.DSeq,
			GSeq:  oldObject.OrderID.GSeq,
			OSeq:  oldObject.OrderID.OSeq,
		},
		State: newMarket.Order_State(oldObject.State),
		Spec: newDeployment.GroupSpec{
			Name:         oldObject.Spec.Name,
			Requirements: newPlacementRequirements,
			Resources:    newResources,
		},
		CreatedAt: oldObject.CreatedAt,
	}

	return newObject, nil
}

func fractionalUaktMigrateEscrowAccount(accountRaw []byte) (newEscrow.Account, error) {
	var oldObject oldEscrow.Account
	err := oldObject.Unmarshal(accountRaw)
	if err != nil {
		return newEscrow.Account{}, err
	}

	newObject := newEscrow.Account{
		ID: newEscrow.AccountID{
			Scope: oldObject.ID.Scope,
			XID:   oldObject.ID.XID,
		},
		Owner: oldObject.Owner,
		State: newEscrow.Account_State(oldObject.State),
		Balance: sdk.DecCoin{
			Denom:  oldObject.Balance.Denom,
			Amount: sdk.NewDecFromInt(oldObject.Balance.Amount),
		},
		Transferred: sdk.DecCoin{
			Denom:  oldObject.Balance.Denom,
			Amount: sdk.NewDecFromInt(oldObject.Transferred.Amount),
		},
		SettledAt: oldObject.SettledAt,
	}
	return newObject, nil
}

func fractionalUaktMigrateEscrowPayment(paymentRaw []byte) (newEscrow.FractionalPayment, error) {
	var oldObject oldEscrow.Payment

	err := oldObject.Unmarshal(paymentRaw)
	if err != nil {
		return newEscrow.FractionalPayment{}, err
	}

	newObject := newEscrow.FractionalPayment{
		AccountID: newEscrow.AccountID{
			Scope: oldObject.AccountID.Scope,
			XID:   oldObject.AccountID.XID,
		},
		PaymentID: oldObject.PaymentID,
		Owner:     oldObject.Owner,
		State:     newEscrow.FractionalPayment_State(oldObject.State),
		Rate: sdk.DecCoin{
			Denom:  oldObject.Rate.Denom,
			Amount: sdk.NewDecFromInt(oldObject.GetRate().Amount),
		},
		Balance: sdk.DecCoin{
			Denom:  oldObject.Balance.Denom,
			Amount: sdk.NewDecFromInt(oldObject.GetBalance().Amount),
		},
		Withdrawn: oldObject.GetWithdrawn(),
	}

	return newObject, nil
}

func fractionalUaktMigrateProvider(providerRaw []byte) (newProvider.Provider, error) {
	var oldObject oldProvider.Provider
	err := oldObject.Unmarshal(providerRaw)
	if err != nil {
		return newProvider.Provider{}, err
	}

	newObject := newProvider.Provider{
		Owner:      oldObject.Owner,
		HostURI:    oldObject.HostURI,
		Attributes: fractionalUaktMigrateAttributes(oldObject.Attributes),
		Info: newProvider.ProviderInfo{
			EMail:   oldObject.Info.EMail,
			Website: oldObject.Info.Website,
		},
	}

	return newObject, nil
}

func fractionalUaktMigrateCertRaw(certRaw []byte) (newCert.Certificate, error) {
	var oldObject oldCert.Certificate
	err := oldObject.Unmarshal(certRaw)
	if err != nil {
		return newCert.Certificate{}, err
	}

	newObject := newCert.Certificate{
		State:  newCert.Certificate_State(oldObject.State),
		Cert:   oldObject.Cert,
		Pubkey: oldObject.Pubkey,
	}

	return newObject, nil
}

func fractionalUaktMigrateAuditProvider(raw []byte) (newAudit.Provider, error) {
	var oldObject oldAudit.Provider
	err := oldObject.Unmarshal(raw)
	if err != nil {
		return newAudit.Provider{}, err
	}

	newObject := newAudit.Provider{
		Owner:      oldObject.Owner,
		Auditor:    oldObject.Auditor,
		Attributes: fractionalUaktMigrateAttributes(oldObject.Attributes),
	}

	return newObject, nil
}
