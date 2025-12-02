package query

import (
	"testing"

	dtypes "pkg.akt.dev/go/node/deployment/v1"
	v1 "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/testutil"
)

func TestGetOrdersPath(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name     string
		filters  OrderFilters
		expected string
	}{
		{
			name: "with owner and state",
			filters: OrderFilters{
				Owner:        owner,
				StateFlagVal: "open",
			},
			expected: "orders/" + owner.String() + "/open",
		},
		{
			name: "empty owner with state",
			filters: OrderFilters{
				StateFlagVal: "closed",
			},
			expected: "orders//closed",
		},
		{
			name: "owner with empty state",
			filters: OrderFilters{
				Owner: owner,
			},
			expected: "orders/" + owner.String() + "/",
		},
		{
			name:     "empty filters",
			filters:  OrderFilters{},
			expected: "orders//",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getOrdersPath(tt.filters)
			if got != tt.expected {
				t.Errorf("getOrdersPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestOrderPath(t *testing.T) {
	owner := testutil.AccAddress(t)
	did := dtypes.DeploymentID{Owner: owner.String(), DSeq: 100}
	gid := dtypes.MakeGroupID(did, 1)
	oid := v1.MakeOrderID(gid, 2)

	expected := "order/" + owner.String() + "/100/1/2"
	got := OrderPath(oid)

	if got != expected {
		t.Errorf("OrderPath() = %v, want %v", got, expected)
	}
}

func TestGetBidsPath(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name     string
		filters  BidFilters
		expected string
	}{
		{
			name: "with owner and state",
			filters: BidFilters{
				Owner:        owner,
				StateFlagVal: "open",
			},
			expected: "bids/" + owner.String() + "/open",
		},
		{
			name: "empty owner with state",
			filters: BidFilters{
				StateFlagVal: "matched",
			},
			expected: "bids//matched",
		},
		{
			name: "owner with empty state",
			filters: BidFilters{
				Owner: owner,
			},
			expected: "bids/" + owner.String() + "/",
		},
		{
			name:     "empty filters",
			filters:  BidFilters{},
			expected: "bids//",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBidsPath(tt.filters)
			if got != tt.expected {
				t.Errorf("getBidsPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetBidPath(t *testing.T) {
	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	did := dtypes.DeploymentID{Owner: owner.String(), DSeq: 100}
	gid := dtypes.MakeGroupID(did, 1)
	oid := v1.MakeOrderID(gid, 2)
	bid := v1.MakeBidID(oid, provider)

	expected := "bid/" + owner.String() + "/100/1/2/" + provider.String()
	got := getBidPath(bid)

	if got != expected {
		t.Errorf("getBidPath() = %v, want %v", got, expected)
	}
}

func TestGetLeasesPath(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name     string
		filters  LeaseFilters
		expected string
	}{
		{
			name: "with owner and state",
			filters: LeaseFilters{
				Owner:        owner,
				StateFlagVal: "active",
			},
			expected: "leases/" + owner.String() + "/active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getLeasesPath(tt.filters)
			if got != tt.expected {
				t.Errorf("getLeasesPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLeasePath(t *testing.T) {
	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	did := dtypes.DeploymentID{Owner: owner.String(), DSeq: 100}
	gid := dtypes.MakeGroupID(did, 1)
	oid := v1.MakeOrderID(gid, 2)
	bid := v1.MakeBidID(oid, provider)
	lid := v1.MakeLeaseID(bid)

	expected := "lease/" + owner.String() + "/100/1/2/" + provider.String()
	got := LeasePath(lid)

	if got != expected {
		t.Errorf("LeasePath() = %v, want %v", got, expected)
	}
}

func TestOrderParts(t *testing.T) {
	owner := testutil.AccAddress(t)
	did := dtypes.DeploymentID{Owner: owner.String(), DSeq: 100}
	gid := dtypes.MakeGroupID(did, 1)
	oid := v1.MakeOrderID(gid, 2)

	expected := owner.String() + "/100/1/2"
	got := orderParts(oid)

	if got != expected {
		t.Errorf("orderParts() = %v, want %v", got, expected)
	}
}

func TestParseOrderPath(t *testing.T) {
	owner := testutil.AccAddress(t)

	tests := []struct {
		name    string
		parts   []string
		wantErr bool
		errType error
	}{
		{
			name:    "valid path",
			parts:   []string{owner.String(), "100", "1", "2"},
			wantErr: false,
		},
		{
			name:    "too few parts",
			parts:   []string{owner.String(), "100", "1"},
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "empty parts",
			parts:   []string{},
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "invalid dseq",
			parts:   []string{owner.String(), "invalid", "1", "2"},
			wantErr: true,
		},
		{
			name:    "invalid gseq",
			parts:   []string{owner.String(), "100", "invalid", "2"},
			wantErr: true,
		},
		{
			name:    "invalid oseq",
			parts:   []string{owner.String(), "100", "1", "invalid"},
			wantErr: true,
		},
		{
			name:    "invalid owner address",
			parts:   []string{"invalidaddress", "100", "1", "2"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOrderPath(tt.parts)

			if tt.wantErr {
				if err == nil {
					t.Error("parseOrderPath() expected error, got nil")
				}
				if tt.errType != nil && err != tt.errType {
					if tt.errType == ErrInvalidPath && err == ErrInvalidPath {
						return
					}
				}
				return
			}

			if err != nil {
				t.Errorf("parseOrderPath() unexpected error: %v", err)
				return
			}

			if got.Owner != owner.String() {
				t.Errorf("parseOrderPath() Owner = %v, want %v", got.Owner, owner.String())
			}
			if got.DSeq != 100 {
				t.Errorf("parseOrderPath() DSeq = %v, want %v", got.DSeq, 100)
			}
			if got.GSeq != 1 {
				t.Errorf("parseOrderPath() GSeq = %v, want %v", got.GSeq, 1)
			}
			if got.OSeq != 2 {
				t.Errorf("parseOrderPath() OSeq = %v, want %v", got.OSeq, 2)
			}
		})
	}
}

func TestParseBidPath(t *testing.T) {
	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	tests := []struct {
		name    string
		parts   []string
		wantErr bool
		errType error
	}{
		{
			name:    "valid path",
			parts:   []string{owner.String(), "100", "1", "2", provider.String()},
			wantErr: false,
		},
		{
			name:    "too few parts",
			parts:   []string{owner.String(), "100", "1", "2"},
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "empty parts",
			parts:   []string{},
			wantErr: true,
			errType: ErrInvalidPath,
		},
		{
			name:    "invalid order parts",
			parts:   []string{owner.String(), "invalid", "1", "2", provider.String()},
			wantErr: true,
		},
		{
			name:    "invalid provider address",
			parts:   []string{owner.String(), "100", "1", "2", "invalidprovider"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBidPath(tt.parts)

			if tt.wantErr {
				if err == nil {
					t.Error("parseBidPath() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseBidPath() unexpected error: %v", err)
				return
			}

			if got.Owner != owner.String() {
				t.Errorf("parseBidPath() Owner = %v, want %v", got.Owner, owner.String())
			}
			if got.DSeq != 100 {
				t.Errorf("parseBidPath() DSeq = %v, want %v", got.DSeq, 100)
			}
			if got.GSeq != 1 {
				t.Errorf("parseBidPath() GSeq = %v, want %v", got.GSeq, 1)
			}
			if got.OSeq != 2 {
				t.Errorf("parseBidPath() OSeq = %v, want %v", got.OSeq, 2)
			}
			if got.Provider != provider.String() {
				t.Errorf("parseBidPath() Provider = %v, want %v", got.Provider, provider.String())
			}
		})
	}
}

func TestParseLeasePath(t *testing.T) {
	owner := testutil.AccAddress(t)
	provider := testutil.AccAddress(t)

	tests := []struct {
		name    string
		parts   []string
		wantErr bool
	}{
		{
			name:    "valid path",
			parts:   []string{owner.String(), "100", "1", "2", provider.String()},
			wantErr: false,
		},
		{
			name:    "too few parts for bid",
			parts:   []string{owner.String(), "100", "1", "2"},
			wantErr: true,
		},
		{
			name:    "empty parts",
			parts:   []string{},
			wantErr: true,
		},
		{
			name:    "invalid bid path",
			parts:   []string{owner.String(), "invalid", "1", "2", provider.String()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLeasePath(tt.parts)

			if tt.wantErr {
				if err == nil {
					t.Error("ParseLeasePath() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLeasePath() unexpected error: %v", err)
				return
			}

			if got.Owner != owner.String() {
				t.Errorf("ParseLeasePath() Owner = %v, want %v", got.Owner, owner.String())
			}
			if got.DSeq != 100 {
				t.Errorf("ParseLeasePath() DSeq = %v, want %v", got.DSeq, 100)
			}
			if got.GSeq != 1 {
				t.Errorf("ParseLeasePath() GSeq = %v, want %v", got.GSeq, 1)
			}
			if got.OSeq != 2 {
				t.Errorf("ParseLeasePath() OSeq = %v, want %v", got.OSeq, 2)
			}
			if got.Provider != provider.String() {
				t.Errorf("ParseLeasePath() Provider = %v, want %v", got.Provider, provider.String())
			}
		})
	}
}
