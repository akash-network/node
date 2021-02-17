package bidengine

import (
	"context"
	"fmt"
	"regexp"

	lifecycle "github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/provider/cluster"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
)

// order manages bidding and general lifecycle handling of an order.
type order struct {
	orderID   mtypes.OrderID
	bidPlaced bool
	cfg       Config

	session                    session.Session
	cluster                    cluster.Cluster
	bus                        pubsub.Bus
	sub                        pubsub.Subscriber
	reservationFulfilledNotify chan<- int

	log log.Logger
	lc  lifecycle.Lifecycle
}

func newOrder(svc *service, oid mtypes.OrderID, cfg Config, checkForExistingBid bool) (*order, error) {
	return newOrderInternal(svc, oid, cfg, checkForExistingBid, nil)
}
func newOrderInternal(svc *service, oid mtypes.OrderID, cfg Config, checkForExistingBid bool, reservationFulfilledNotify chan<- int) (*order, error) {
	// Create a subscription that will see all events that have not been read from e.sub.Events()
	sub, err := svc.sub.Clone()
	if err != nil {
		return nil, err
	}

	session := svc.session.ForModule("bidengine-order")

	log := session.Log().With("order", oid)

	order := &order{
		cfg:                        cfg,
		orderID:                    oid,
		bidPlaced:                  false,
		session:                    session,
		cluster:                    svc.cluster,
		bus:                        svc.bus,
		sub:                        sub,
		log:                        log,
		lc:                         lifecycle.New(),
		reservationFulfilledNotify: reservationFulfilledNotify, // Normally nil in production
	}

	// Shut down when parent begins shutting down
	go order.lc.WatchChannel(svc.lc.ShuttingDown())

	// Run main loop in separate thread.
	go order.run(checkForExistingBid)

	// Notify parent of completion (allows drain).
	go func() {
		<-order.lc.Done()
		svc.drainch <- order
	}()

	return order, nil
}

var matchBidNotFound = regexp.MustCompile("^.+bid not found.+$")

func (o *order) run(checkForExistingBid bool) {
	defer o.lc.ShutdownCompleted()
	ctx, cancel := context.WithCancel(context.Background())

	var (
		// channels for async operations.
		groupch       <-chan runner.Result
		storedGroupCh <-chan runner.Result
		clusterch     <-chan runner.Result
		bidch         <-chan runner.Result
		pricech       <-chan runner.Result
		queryBidCh    <-chan runner.Result

		group       *dtypes.Group
		reservation ctypes.Reservation

		won bool
		msg *mtypes.MsgCreateBid
	)

	// Begin fetching group details immediately.
	groupch = runner.Do(func() runner.Result {
		res, err := o.session.Client().Query().Group(ctx, &dtypes.QueryGroupRequest{ID: o.orderID.GroupID()})
		return runner.NewResult(res.GetGroup(), err)
	})

	// Load existing bid if needed
	if checkForExistingBid {
		queryBidCh = runner.Do(func() runner.Result {
			return runner.NewResult(o.session.Client().Query().Bid(
				ctx,
				&mtypes.QueryBidRequest{
					ID: mtypes.MakeBidID(o.orderID, o.session.Provider().Address()),
				},
			))
		})
		// Hide the group details result for later
		storedGroupCh = groupch
		groupch = nil
	}
loop:
	for {
		select {
		case <-o.lc.ShutdownRequest():
			break loop

		case queryBid := <-queryBidCh:
			err := queryBid.Error()
			bidFound := true
			if err != nil {
				if matchBidNotFound.MatchString(err.Error()) {
					bidFound = false
				} else {
					o.session.Log().Error("could not get existing bid", "err", err, "errtype", fmt.Sprintf("%T", err))
					break loop
				}
			}

			if bidFound {
				o.bidPlaced = true
				o.session.Log().Info("Found existing bid ")
			}
			groupch = storedGroupCh // Allow getting the group details result now
			storedGroupCh = nil

		case ev := <-o.sub.Events():
			switch ev := ev.(type) {
			case mtypes.EventLeaseCreated:

				// different group
				if !o.orderID.GroupID().Equals(ev.ID.GroupID()) {
					o.log.Debug("ignoring group", "group", ev.ID.GroupID())
					break
				}

				// check winning provider
				if ev.ID.Provider != o.session.Provider().Address().String() {
					o.log.Info("lease lost", "lease", ev.ID)
					break loop
				}

				// TODO: sanity check (price, state, etc...)
				o.log.Info("lease won", "lease", ev.ID)

				if err := o.bus.Publish(event.LeaseWon{
					LeaseID: ev.ID,
					Group:   group,
					Price:   ev.Price,
				}); err != nil {
					o.log.Error("failed to publish to event queue", err)
				}
				won = true

				break loop

			case mtypes.EventOrderClosed:

				// different deployment
				if !ev.ID.Equals(o.orderID) {
					break
				}

				o.log.Info("order closed")
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

			res := result.Value().(dtypes.Group)
			group = &res

			if !o.shouldBid(group) {
				break loop
			}

			o.log.Info("requesting reservation")
			// Begin reserving resources from cluster.
			clusterch = runner.Do(func() runner.Result {
				v := runner.NewResult(o.cluster.Reserve(o.orderID, group))
				return v
			})

		case result := <-clusterch:
			clusterch = nil

			if result.Error() != nil {
				o.log.Error("reserving resources", "err", result.Error())
				break loop
			}

			o.log.Info("Reservation fulfilled")

			// If the channel is assigned and there is capacity, write into the channel
			if o.reservationFulfilledNotify != nil {
				select {
				case o.reservationFulfilledNotify <- 0:
				default:
				}
			}

			// Resources reserved.
			reservation = result.Value().(ctypes.Reservation)
			if o.bidPlaced {
				o.log.Info("Fulfillment already exists")
				// fulfillment already created (state recovered via queryExistingOrders)
				break
			}

			pricech = runner.Do(func() runner.Result {
				// Calculate price & bid
				return runner.NewResult(o.cfg.PricingStrategy.calculatePrice(ctx, &group.GroupSpec))

			})
		case result := <-pricech:
			pricech = nil
			if result.Error() != nil {
				o.log.Error("error calculating price", "err", result.Error())
				break loop
			}
			price := result.Value().(sdk.Coin)
			o.log.Debug("submitting fulfillment", "price", price)

			// Begin submitting fulfillment
			msg = mtypes.NewMsgCreateBid(o.orderID, o.session.Provider().Address(), price, o.cfg.Deposit)
			bidch = runner.Do(func() runner.Result {
				return runner.NewResult(nil, o.session.Client().Tx().Broadcast(ctx, msg))
			})

		case result := <-bidch:
			bidch = nil
			o.log.Info("bid complete")

			if result.Error() != nil {
				o.log.Error("submitting fulfillment", "err", result.Error())
				break loop
			}

			// Fulfillment placed.
			o.bidPlaced = true
		}
	}

	o.log.Info("shutting down")
	cancel()
	o.lc.ShutdownInitiated(nil)
	o.sub.Close()

	// cancel reservation
	if !won {
		if reservation != nil {
			o.log.Debug("unreserving reservation")
			if err := o.cluster.Unreserve(reservation.OrderID()); err != nil {
				o.log.Error("error unreserving reservation", "err", err)
			}
		}

		if o.bidPlaced {
			o.log.Debug("closing bid")
			err := o.session.Client().Tx().Broadcast(ctx, &mtypes.MsgCloseBid{
				BidID: mtypes.MakeBidID(o.orderID, o.session.Provider().Address()),
			})
			if err != nil {
				o.log.Error("closing bid", "err", err)
			}
		}
	}

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
	if pricech != nil {
		<-pricech
	}
}

func (o *order) shouldBid(group *dtypes.Group) bool {

	// does provider have required attributes?
	// fixme - MatchAttributes does not check for signed attributes
	// it is done during processing of MsgCreateBid
	if !group.GroupSpec.MatchAttributes(o.session.Provider().Attributes) {
		o.log.Debug("unable to fulfill: incompatible attributes")
		return false
	}

	if err := group.GroupSpec.ValidateBasic(); err != nil {
		o.log.Error("unable to fulfill: group validation error",
			"err", err)
		return false
	}
	return true
}
