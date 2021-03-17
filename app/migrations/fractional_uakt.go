package migrations

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	audit "github.com/ovrclk/akash/x/audit/keeper"
	newAudit "github.com/ovrclk/akash/x/audit/types"
	cert "github.com/ovrclk/akash/x/cert/keeper"
	deployment "github.com/ovrclk/akash/x/deployment/keeper"
	escrow "github.com/ovrclk/akash/x/escrow/keeper"
	market "github.com/ovrclk/akash/x/market/keeper"
	provider "github.com/ovrclk/akash/x/provider/keeper"
	"math/big"

	newDeployment "github.com/ovrclk/akash/x/deployment/types"
)

func MigrateFractionalUakt(
	ctx sdk.Context,
	akeeper audit.Keeper,
	ckeeper cert.Keeper,
	dkeeper deployment.Keeper,
	ekeeper escrow.Keeper,
	mkeeper market.Keeper,
	pkeeper provider.Keeper,
) {

	logger := ctx.Logger().With("migration", "fractional-uakt")
	dkeeper.IterateDeploymentsRaw(ctx, func(deploymentRaw []byte) bool {
		newObj, err := fractionalUaktMigrateDeployment(deploymentRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating deployment", "deployment-id", newObj.DeploymentID)

		err = dkeeper.UpdateDeployment(ctx, newObj)
		if err != nil {
			panic(err)
		}

		return false
	})

	dkeeper.IterateGroupsRaw(ctx, func(id newDeployment.GroupID, groupRaw []byte) bool {
		newObj, err := fractionalUaktMigrateGroup(groupRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating group", "group-id", id)

		dkeeper.SetGroup(ctx, id, newObj)
		return false
	})

	mkeeper.IterateLeasesRaw(ctx, func(leaseRaw []byte) bool {
		newObj, err := fractionalUaktMigrateLease(leaseRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating lease", "lease-id", newObj.LeaseID)

		mkeeper.SetLease(ctx, newObj)
		return false
	})

	mkeeper.IterateBidsRaw(ctx, func(bidRaw []byte) bool {
		newObj, err := fractionalUaktMigrateBid(bidRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating bid", "bid-id", newObj.BidID)

		err = mkeeper.SetBid(ctx, newObj, false)
		if err != nil {
			panic(err)
		}
		return false
	})

	mkeeper.IterateOrdersRaw(ctx, func(orderRaw []byte) bool {
		newObj, err := fractionalUaktMigrateOrder(orderRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating order", "order-id", newObj.OrderID)
		logger.Debug("Spec price", "order-id", newObj.OrderID, "price", newObj.Spec.Price())
		for i, resource := range newObj.Spec.Resources {
			logger.Debug("Price: %s Count: %d\n", "order-id", newObj.OrderID, "index", i, "price", resource.Price, "count", resource.Count)
		}

		err = mkeeper.SetOrder(ctx, newObj, false)
		if err != nil {
			panic(err)
		}
		return false
	})

	ekeeper.IterateAccountsRaw(ctx, func(accountRaw []byte) bool {
		newObj, err := fractionalUaktMigrateEscrowAccount(accountRaw)
		if err != nil {
			panic(err)
		}

		logger.Debug("Updating escrow account", "id", newObj.ID)
		ekeeper.SaveAccount(ctx, newObj)
		return false
	})

	ekeeper.IteratePaymentsRaw(ctx, func(paymentRaw []byte) bool {
		newObj, err := fractionalUaktMigrateEscrowPayment(paymentRaw)
		if err != nil {
			panic(err)
		}

		logger.Debug("Updating payment for escrow account", "account-id", newObj.AccountID, "balance", newObj.Balance)
		ekeeper.SavePayment(ctx, newObj)
		return false
	})

	pkeeper.IterateProvidersRaw(ctx, func(providerRaw []byte) bool {
		newObj, err := fractionalUaktMigrateProvider(providerRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating provider", "owner", newObj.Owner)
		err = pkeeper.Update(ctx, newObj)
		if err != nil {
			panic(err)
		}
		return false
	})

	ckeeper.IterateCertificatesRaw(ctx, func(owner sdk.Address, serial big.Int, certRaw []byte) bool {
		newObj, err := fractionalUaktMigrateCertRaw(certRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating cert", "owner", owner, "serial", serial, "state", newObj.State)

		ckeeper.SetCertificate(ctx, owner, serial, newObj)

		return false
	})

	akeeper.IterateProvidersRaw(ctx, func(providerAttributesRaw []byte) bool {
		newObj, err := fractionalUaktMigrateAuditProvider(providerAttributesRaw)
		if err != nil {
			panic(err)
		}
		logger.Debug("Updating audit of", "auditor", newObj.Auditor, "owner", newObj.Owner)

		ownerAddr, err := sdk.AccAddressFromBech32(newObj.Owner)
		if err != nil {
			panic(err)
		}
		auditorAddr, err := sdk.AccAddressFromBech32(newObj.Auditor)
		if err != nil {
			panic(err)
		}
		id := newAudit.ProviderID{
			Owner:   ownerAddr,
			Auditor: auditorAddr,
		}
		err = akeeper.CreateOrUpdateProviderAttributes(ctx, id, newObj.Attributes)
		if err != nil {
			panic(err)
		}

		return false
	})
}
