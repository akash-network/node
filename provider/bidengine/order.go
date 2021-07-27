package bidengine

import (
	"context"
	"fmt"
	"regexp"
	"time"

	lifecycle "github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/provider/cluster"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	"github.com/ovrclk/akash/util/runner"
	atypes "github.com/ovrclk/akash/x/audit/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

	log  log.Logger
	lc   lifecycle.Lifecycle
	pass ProviderAttrSignatureService
}

var (
	pricingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:        "provider_bid_pricing_duration",
		Help:        "",
		ConstLabels: nil,
		Buckets:     prometheus.ExponentialBuckets(150000.0, 2.0, 10.0),
	})

	bidCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_bid",
		Help: "The total number of bids created",
	}, []string{"action", "result"})

	reservationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:        "provider_reservation_duration",
		Help:        "",
		ConstLabels: nil,
		Buckets:     prometheus.ExponentialBuckets(150000.0, 2.0, 10.0),
	})

	reservationCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_reservation",
		Help: "",
	}, []string{"action", "result"})

	shouldBidCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_should_bid",
		Help: "",
	}, []string{"result"})

	orderCompleteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_order_complete",
		Help: "",
	}, []string{"result"})
)

func newOrder(svc *service, oid mtypes.OrderID, cfg Config, pass ProviderAttrSignatureService, checkForExistingBid bool) (*order, error) {
	return newOrderInternal(svc, oid, cfg, pass, checkForExistingBid, nil)
}
func newOrderInternal(svc *service, oid mtypes.OrderID, cfg Config, pass ProviderAttrSignatureService, checkForExistingBid bool, reservationFulfilledNotify chan<- int) (*order, error) {
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
		pass:                       pass,
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
		shouldBidCh   <-chan runner.Result
		bidTimeout    <-chan time.Time

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
					orderCompleteCounter.WithLabelValues("lease-lost").Inc()
					o.log.Info("lease lost", "lease", ev.ID)
					break loop
				}
				orderCompleteCounter.WithLabelValues("lease-won").Inc()

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
				orderCompleteCounter.WithLabelValues("order-closed").Inc()
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

			shouldBidCh = runner.Do(func() runner.Result {
				return runner.NewResult(o.shouldBid(group))
			})

		case result := <-shouldBidCh:
			shouldBidCh = nil

			if result.Error() != nil {
				shouldBidCounter.WithLabelValues(metricsutils.FailLabel).Inc()
				o.log.Error("failure during checking should bid", "err", result.Error())
				break loop
			}

			shouldBid := result.Value().(bool)
			if !shouldBid {
				shouldBidCounter.WithLabelValues("decline").Inc()
				o.log.Debug("declined to bid")
				break loop
			}

			shouldBidCounter.WithLabelValues("accept").Inc()
			o.log.Info("requesting reservation")
			// Begin reserving resources from cluster.
			clusterch = runner.Do(metricsutils.ObserveRunner(func() runner.Result {
				v := runner.NewResult(o.cluster.Reserve(o.orderID, group))
				return v
			}, reservationDuration))

		case result := <-clusterch:
			clusterch = nil

			if result.Error() != nil {
				reservationCounter.WithLabelValues(metricsutils.OpenLabel, metricsutils.FailLabel)
				o.log.Error("reserving resources", "err", result.Error())
				break loop
			}

			reservationCounter.WithLabelValues(metricsutils.OpenLabel, metricsutils.SuccessLabel)

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
			pricech = runner.Do(metricsutils.ObserveRunner(func() runner.Result {
				// Calculate price & bid
				return runner.NewResult(o.cfg.PricingStrategy.CalculatePrice(ctx, group.GroupID.Owner, &group.GroupSpec))
			}, pricingDuration))
		case result := <-pricech:
			pricech = nil
			if result.Error() != nil {
				o.log.Error("error calculating price", "err", result.Error())
				break loop
			}

			price := result.Value().(sdk.Coin)
			maxPrice := group.GroupSpec.Price()

			if maxPrice.IsLT(price) {
				o.log.Info("Price too high, not bidding", "price", price.String(), "max-price", maxPrice.String())
				break loop
			}

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
				bidCounter.WithLabelValues(metricsutils.OpenLabel, metricsutils.FailLabel).Inc()
				o.log.Error("submitting fulfillment", "err", result.Error())
				break loop
			}
			bidCounter.WithLabelValues(metricsutils.OpenLabel, metricsutils.SuccessLabel).Inc()

			// Fulfillment placed.
			o.bidPlaced = true

			if o.cfg.BidTimeout > time.Duration(0) {
				bidTimeout = time.After(o.cfg.BidTimeout)
			}

		case <-bidTimeout:
			o.log.Info("bid timeout, closing bid")
			orderCompleteCounter.WithLabelValues("bid-timeout").Inc()
			break loop
		}
	}

	o.log.Info("shutting down")
	o.lc.ShutdownInitiated(nil)
	o.sub.Close()

	// cancel reservation
	if !won {
		if clusterch != nil {
			result := <-clusterch
			clusterch = nil
			if result.Error() == nil {
				reservation = result.Value().(ctypes.Reservation)
			}
		}
		if reservation != nil {
			o.log.Debug("unreserving reservation")
			if err := o.cluster.Unreserve(reservation.OrderID()); err != nil {
				o.log.Error("error unreserving reservation", "err", err)
				reservationCounter.WithLabelValues("close", metricsutils.FailLabel)
			} else {
				reservationCounter.WithLabelValues("close", metricsutils.SuccessLabel)
			}
		}

		if o.bidPlaced {
			o.log.Debug("closing bid")
			err := o.session.Client().Tx().Broadcast(ctx, &mtypes.MsgCloseBid{
				BidID: mtypes.MakeBidID(o.orderID, o.session.Provider().Address()),
			})
			if err != nil {
				o.log.Error("closing bid", "err", err)
				bidCounter.WithLabelValues("close", metricsutils.FailLabel).Inc()
			} else {
				bidCounter.WithLabelValues("close", metricsutils.SuccessLabel).Inc()
			}
		}
	}
	cancel()

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

func (o *order) shouldBid(group *dtypes.Group) (bool, error) {

	// does provider have required attributes?
	if !group.GroupSpec.MatchAttributes(o.session.Provider().Attributes) {
		o.log.Debug("unable to fulfill: incompatible attributes")
		return false, nil
	}

	signatureRequirements := group.GroupSpec.Requirements.SignedBy
	if signatureRequirements.Size() != 0 {
		// Check that the signature requirements are met for each attribute
		var provAttr []atypes.Provider
		ownAttrs := atypes.Provider{
			Owner:      o.session.Provider().Owner,
			Auditor:    "",
			Attributes: o.session.Provider().Attributes,
		}
		provAttr = append(provAttr, ownAttrs)
		auditors := make([]string, 0)
		auditors = append(auditors, group.GroupSpec.Requirements.SignedBy.AllOf...)
		auditors = append(auditors, group.GroupSpec.Requirements.SignedBy.AnyOf...)

		gotten := make(map[string]struct{})
		for _, auditor := range auditors {
			_, done := gotten[auditor]
			if done {
				continue
			}
			result, err := o.pass.GetAuditorAttributeSignatures(auditor)
			if err != nil {
				return false, err
			}
			provAttr = append(provAttr, result...)
			gotten[auditor] = struct{}{}
		}

		ok := group.GroupSpec.MatchRequirements(provAttr)
		if !ok {
			o.log.Debug("attribute signature requirements not met")
			return false, nil
		}
	}

	if err := group.GroupSpec.ValidateBasic(); err != nil {
		o.log.Error("unable to fulfill: group validation error",
			"err", err)
		return false, nil
	}
	return true, nil
}
