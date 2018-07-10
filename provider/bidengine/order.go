package bidengine

import (
	"bytes"
	"context"
	"math/rand"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/runner"
	"github.com/tendermint/tmlibs/log"
)

// order manages bidding and general lifecycle handling of an order.
type order struct {
	order       types.OrderID
	fulfillment *types.Fulfillment

	session session.Session
	cluster cluster.Cluster
	bus     event.Bus
	sub     event.Subscriber

	log log.Logger
	lc  lifecycle.Lifecycle
}

func newOrder(e *service, oid types.OrderID, fulfillment *types.Fulfillment) (*order, error) {

	// Create a subscription that will see all events that have not been read from e.sub.Events()
	sub, err := e.sub.Clone()
	if err != nil {
		return nil, err
	}

	session := e.session.ForModule("bidengine-order")

	log := session.Log().With("order", oid)

	order := &order{
		order:       oid,
		fulfillment: fulfillment,
		session:     session,
		cluster:     e.cluster,
		bus:         e.bus,
		sub:         sub,
		log:         log,
		lc:          lifecycle.New(),
	}

	// Shut down when parent begins shutting down
	go order.lc.WatchChannel(e.lc.ShuttingDown())

	// Run main loop in separate thread.
	go order.run()

	// Notify parent of completion (allows drain).
	go func() {
		<-order.lc.Done()
		e.drainch <- order
	}()

	return order, nil
}

func (o *order) run() {
	defer o.lc.ShutdownCompleted()

	ctx, cancel := context.WithCancel(context.Background())

	var (
		// channels for async operations.

		groupch   <-chan runner.Result
		clusterch <-chan runner.Result
		bidch     <-chan runner.Result

		group       *types.DeploymentGroup
		reservation cluster.Reservation
	)

	// Begin fetching group details immediately.
	groupch = runner.Do(func() runner.Result {
		return runner.NewResult(
			o.session.Query().DeploymentGroup(ctx, o.order.GroupID()))
	})

loop:
	for {
		select {
		case <-o.lc.ShutdownRequest():
			break loop

		case ev := <-o.sub.Events():
			switch ev := ev.(type) {
			case *event.TxCreateLease:

				// different group
				if o.order.GroupID().Compare(ev.GroupID()) != 0 {
					o.log.Debug("ignoring group", "group", ev.GroupID())
					break
				}

				// check winning provider
				if !bytes.Equal(o.session.Provider().Address, ev.Provider) {
					o.log.Info("lease lost", "lease", ev.LeaseID)
					break loop
				}

				// TODO: sanity check (price, state, etc...)

				o.log.Info("lease won", "lease", ev.LeaseID, "price", ev.Price)

				o.bus.Publish(event.LeaseWon{
					LeaseID: ev.LeaseID,
					Group:   group,
					Price:   ev.Price,
				})

				break loop

			case *event.TxCloseDeployment:

				// different deployment
				if !bytes.Equal(o.order.Deployment, ev.Deployment) {
					break
				}

				o.log.Info("deployment closed")
				break loop
			}

		case result := <-groupch:
			// Group details fetched.

			groupch = nil
			o.log.Info("group fetched")

			if result.Error() != nil {
				o.log.Error("fetching group", "err", result.Error())
				break loop
			}

			group = result.Value().(*types.DeploymentGroup)

			if !matchProviderAttributes(o.session.Provider().Attributes, group.Requirements) {
				o.log.Debug("unable to fulfill: incompatible attributes")
				break loop
			}

			// TODO: check if price is too low

			// Begin reserving resources from cluster.
			clusterch = runner.Do(func() runner.Result {
				return runner.NewResult(o.cluster.Reserve(o.order, group))
			})

		case result := <-clusterch:
			clusterch = nil
			o.log.Info("reserve requested")

			if result.Error() != nil {
				o.log.Error("reserving resources", "err", result.Error())
				break loop
			}

			if o.fulfillment != nil {
				// fulfillment already created (state recovered via queryExistingOrders)
				break
			}

			// Resources reservied.  Calculate price and bid.

			reservation = result.Value().(cluster.Reservation)

			price := o.calculatePrice(reservation.Group())

			o.log.Debug("submitting fulfillment", "price", price)

			// Begin submitting fulfillment
			bidch = runner.Do(func() runner.Result {
				return runner.NewResult(o.session.TX().BroadcastTxCommit(&types.TxCreateFulfillment{
					FulfillmentID: types.FulfillmentID{
						Deployment: o.order.Deployment,
						Group:      o.order.Group,
						Order:      o.order.Seq,
						Provider:   o.session.Provider().Address,
					},
					Price: price,
				}))
			})

		case result := <-bidch:
			bidch = nil
			o.log.Info("bid complete")

			if result.Error() != nil {
				o.log.Error("submitting fulfillment", "err", result.Error())
				break loop
			}

			// Fulfillment placed.  All done.
		}
	}

	// TODO: cancel reservation?

	o.log.Info("shutting down")
	cancel()
	o.lc.ShutdownInitiated(nil)
	o.sub.Close()

	// Wait for all runners to complete.
	if groupch != nil {
		<-groupch
	}
	if clusterch != nil {
		<-clusterch
	}
	if bidch != nil {
		<-bidch
	}
}

func (o *order) calculatePrice(group *types.DeploymentGroup) uint64 {
	max := o.groupMaxPrice(group)
	return uint64(rand.Int63n(int64(max)) + 1)
}

func (o *order) groupMaxPrice(group *types.DeploymentGroup) uint64 {
	// TODO: catch overflow
	price := uint64(0)
	for _, group := range group.GetResources() {
		price += group.Price
	}
	o.log.Debug("group max price", "price", price)
	return price
}
