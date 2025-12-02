package query

import (
	"strings"
	"testing"

	"pkg.akt.dev/go/testutil"
)

func TestProviderString(t *testing.T) {
	owner := testutil.AccAddress(t)

	provider := Provider{
		Owner:   owner.String(),
		HostURI: "https://provider.example.com",
	}

	got := provider.String()

	if !strings.Contains(got, owner.String()) {
		t.Errorf("Provider.String() should contain owner, got %v", got)
	}
	if !strings.Contains(got, "https://provider.example.com") {
		t.Errorf("Provider.String() should contain HostURI, got %v", got)
	}
}

func TestProvidersString(t *testing.T) {
	owner1 := testutil.AccAddress(t)
	owner2 := testutil.AccAddress(t)

	providers := Providers{
		{
			Owner:   owner1.String(),
			HostURI: "https://provider1.example.com",
		},
		{
			Owner:   owner2.String(),
			HostURI: "https://provider2.example.com",
		},
	}

	got := providers.String()

	if !strings.Contains(got, owner1.String()) {
		t.Errorf("Providers.String() should contain first owner, got %v", got)
	}
	if !strings.Contains(got, owner2.String()) {
		t.Errorf("Providers.String() should contain second owner, got %v", got)
	}
}

func TestProvidersStringEmpty(t *testing.T) {
	providers := Providers{}
	got := providers.String()

	if got != "" {
		t.Errorf("Providers.String() for empty slice = %v, want empty string", got)
	}
}

func TestProvidersStringSingle(t *testing.T) {
	owner := testutil.AccAddress(t)

	providers := Providers{
		{
			Owner:   owner.String(),
			HostURI: "https://provider.example.com",
		},
	}

	got := providers.String()

	if !strings.Contains(got, owner.String()) {
		t.Errorf("Providers.String() should contain owner, got %v", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("Providers.String() should not end with separator for single item")
	}
}

func TestProviderAddress(t *testing.T) {
	owner := testutil.AccAddress(t)

	provider := Provider{
		Owner: owner.String(),
	}

	got := provider.Address()

	if !got.Equals(owner) {
		t.Errorf("Provider.Address() = %v, want %v", got, owner)
	}
}

func TestProviderAddressPanicsOnInvalidOwner(t *testing.T) {
	provider := Provider{
		Owner: "invalid-address",
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Provider.Address() should panic on invalid owner address")
		}
	}()

	_ = provider.Address()
}

