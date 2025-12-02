package query

import (
	"testing"

	v1 "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"
	"pkg.akt.dev/go/testutil"
)

func TestOrderFiltersAccept(t *testing.T) {
	owner := testutil.AccAddress(t)
	otherOwner := testutil.AccAddress(t)

	order := v1beta5.Order{
		ID: v1.OrderID{
			Owner: owner.String(),
			DSeq:  100,
			GSeq:  1,
			OSeq:  1,
		},
		State: v1beta5.OrderOpen,
	}

	tests := []struct {
		name         string
		filters      OrderFilters
		order        v1beta5.Order
		isValidState bool
		expected     bool
	}{
		{
			name:         "empty owner and invalid state accepts all",
			filters:      OrderFilters{},
			order:        order,
			isValidState: false,
			expected:     true,
		},
		{
			name: "empty owner with matching state",
			filters: OrderFilters{
				State: v1beta5.OrderOpen,
			},
			order:        order,
			isValidState: true,
			expected:     true,
		},
		{
			name: "empty owner with non-matching state",
			filters: OrderFilters{
				State: v1beta5.OrderActive,
			},
			order:        order,
			isValidState: true,
			expected:     false,
		},
		{
			name: "matching owner with invalid state",
			filters: OrderFilters{
				Owner: owner,
			},
			order:        order,
			isValidState: false,
			expected:     true,
		},
		{
			name: "non-matching owner with invalid state",
			filters: OrderFilters{
				Owner: otherOwner,
			},
			order:        order,
			isValidState: false,
			expected:     false,
		},
		{
			name: "matching owner and matching state",
			filters: OrderFilters{
				Owner: owner,
				State: v1beta5.OrderOpen,
			},
			order:        order,
			isValidState: true,
			expected:     true,
		},
		{
			name: "matching owner but non-matching state",
			filters: OrderFilters{
				Owner: owner,
				State: v1beta5.OrderActive,
			},
			order:        order,
			isValidState: true,
			expected:     false,
		},
		{
			name: "non-matching owner with matching state",
			filters: OrderFilters{
				Owner: otherOwner,
				State: v1beta5.OrderOpen,
			},
			order:        order,
			isValidState: true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filters.Accept(tt.order, tt.isValidState)
			if got != tt.expected {
				t.Errorf("OrderFilters.Accept() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBidFiltersAccept(t *testing.T) {
	owner := testutil.AccAddress(t)
	otherOwner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	bid := v1beta5.Bid{
		ID: v1.BidID{
			Owner:    owner.String(),
			DSeq:     100,
			GSeq:     1,
			OSeq:     1,
			Provider: provider.String(),
		},
		State: v1beta5.BidOpen,
	}

	tests := []struct {
		name         string
		filters      BidFilters
		bid          v1beta5.Bid
		isValidState bool
		expected     bool
	}{
		{
			name:         "empty owner and invalid state accepts all",
			filters:      BidFilters{},
			bid:          bid,
			isValidState: false,
			expected:     true,
		},
		{
			name: "empty owner with matching state",
			filters: BidFilters{
				State: v1beta5.BidOpen,
			},
			bid:          bid,
			isValidState: true,
			expected:     true,
		},
		{
			name: "empty owner with non-matching state",
			filters: BidFilters{
				State: v1beta5.BidActive,
			},
			bid:          bid,
			isValidState: true,
			expected:     false,
		},
		{
			name: "matching owner with invalid state",
			filters: BidFilters{
				Owner: owner,
			},
			bid:          bid,
			isValidState: false,
			expected:     true,
		},
		{
			name: "non-matching owner with invalid state",
			filters: BidFilters{
				Owner: otherOwner,
			},
			bid:          bid,
			isValidState: false,
			expected:     false,
		},
		{
			name: "matching owner and matching state",
			filters: BidFilters{
				Owner: owner,
				State: v1beta5.BidOpen,
			},
			bid:          bid,
			isValidState: true,
			expected:     true,
		},
		{
			name: "matching owner but non-matching state",
			filters: BidFilters{
				Owner: owner,
				State: v1beta5.BidActive,
			},
			bid:          bid,
			isValidState: true,
			expected:     false,
		},
		{
			name: "non-matching owner with matching state",
			filters: BidFilters{
				Owner: otherOwner,
				State: v1beta5.BidOpen,
			},
			bid:          bid,
			isValidState: true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filters.Accept(tt.bid, tt.isValidState)
			if got != tt.expected {
				t.Errorf("BidFilters.Accept() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLeaseFiltersAccept(t *testing.T) {
	owner := testutil.AccAddress(t)
	otherOwner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	lease := v1.Lease{
		ID: v1.LeaseID{
			Owner:    owner.String(),
			DSeq:     100,
			GSeq:     1,
			OSeq:     1,
			Provider: provider.String(),
		},
		State: v1.LeaseActive,
	}

	tests := []struct {
		name         string
		filters      LeaseFilters
		lease        v1.Lease
		isValidState bool
		expected     bool
	}{
		{
			name:         "empty owner and invalid state accepts all",
			filters:      LeaseFilters{},
			lease:        lease,
			isValidState: false,
			expected:     true,
		},
		{
			name: "empty owner with matching state",
			filters: LeaseFilters{
				State: v1.LeaseActive,
			},
			lease:        lease,
			isValidState: true,
			expected:     true,
		},
		{
			name: "empty owner with non-matching state",
			filters: LeaseFilters{
				State: v1.LeaseClosed,
			},
			lease:        lease,
			isValidState: true,
			expected:     false,
		},
		{
			name: "matching owner with invalid state",
			filters: LeaseFilters{
				Owner: owner,
			},
			lease:        lease,
			isValidState: false,
			expected:     true,
		},
		{
			name: "non-matching owner with invalid state",
			filters: LeaseFilters{
				Owner: otherOwner,
			},
			lease:        lease,
			isValidState: false,
			expected:     false,
		},
		{
			name: "matching owner and matching state",
			filters: LeaseFilters{
				Owner: owner,
				State: v1.LeaseActive,
			},
			lease:        lease,
			isValidState: true,
			expected:     true,
		},
		{
			name: "matching owner but non-matching state",
			filters: LeaseFilters{
				Owner: owner,
				State: v1.LeaseClosed,
			},
			lease:        lease,
			isValidState: true,
			expected:     false,
		},
		{
			name: "non-matching owner with matching state",
			filters: LeaseFilters{
				Owner: otherOwner,
				State: v1.LeaseActive,
			},
			lease:        lease,
			isValidState: true,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filters.Accept(tt.lease, tt.isValidState)
			if got != tt.expected {
				t.Errorf("LeaseFilters.Accept() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestOrderString(t *testing.T) {
	order := Order{}
	got := order.String()
	if got != todo {
		t.Errorf("Order.String() = %v, want %v", got, todo)
	}
}

func TestOrdersString(t *testing.T) {
	orders := Orders{}
	got := orders.String()
	if got != todo {
		t.Errorf("Orders.String() = %v, want %v", got, todo)
	}
}

func TestBidString(t *testing.T) {
	bid := Bid{}
	got := bid.String()
	if got != todo {
		t.Errorf("Bid.String() = %v, want %v", got, todo)
	}
}

func TestBidsString(t *testing.T) {
	bids := Bids{}
	got := bids.String()
	if got != todo {
		t.Errorf("Bids.String() = %v, want %v", got, todo)
	}
}

func TestLeaseString(t *testing.T) {
	lease := Lease{}
	got := lease.String()
	if got != todo {
		t.Errorf("Lease.String() = %v, want %v", got, todo)
	}
}

func TestLeasesString(t *testing.T) {
	leases := Leases{}
	got := leases.String()
	if got != todo {
		t.Errorf("Leases.String() = %v, want %v", got, todo)
	}
}

