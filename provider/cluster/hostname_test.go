package cluster

import (
	"context"
	"errors"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type scaffold struct {
	service *hostnameService
	ctx     context.Context
	cancel  context.CancelFunc
}
const testWait = time.Second * time.Duration(15)

func makeHostnameScaffold(t *testing.T, blockedHostnames []string) *scaffold {
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	svc, err := newHostnameService(ctx, Config{BlockedHostnames: blockedHostnames}, nil)
	require.NoError(t, err)

	v := &scaffold{
		service: svc,
		ctx:     ctx,
		cancel:  cancel,
	}

	return v
}



func TestBlockedHostname(t *testing.T) {
	s := makeHostnameScaffold(t, []string{"foobar.com", "bobsdefi.com"})

	ownerAddr := testutil.AccAddress(t)
	err := s.service.CanReserveHostnames([]string{"foobar.com", "other.org"}, ownerAddr)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHostnameNotAllowed))
	require.Regexp(t, "^.*blocked by this provider.*$", err.Error())

	s.cancel()
	select {
	case <-s.service.lc.Done():

	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestBlockedDomain(t *testing.T) {
	s := makeHostnameScaffold(t, []string{"foobar.com", ".bobsdefi.com"})

	ownerAddr := testutil.AccAddress(t)
	err := s.service.CanReserveHostnames([]string{"accounts.bobsdefi.com"}, ownerAddr)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHostnameNotAllowed))
	require.Regexp(t, "^.*blocked by this provider.*$", err.Error())

	s.cancel()
	select {
	case <-s.service.lc.Done():

	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestReserveMoreHostnamesSameDeployment(t *testing.T) {
	s := makeHostnameScaffold(t, []string{"foobar.com", ".bobsdefi.com"})

	leaseID := testutil.LeaseID(t)
	result, err := s.service.ReserveHostnames(s.ctx, []string{"meow.com", "kittens.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	result, err = s.service.ReserveHostnames(s.ctx, []string{"kittens.com", "meow.com", "cats.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 0) // Not withheld because it's the same lease

	secondLeaseID := testutil.LeaseID(t)
	secondLeaseID.Owner = leaseID.Owner
	result, err = s.service.ReserveHostnames(s.ctx, []string{"dogs.com", "meow.com", "ferrets.com"}, secondLeaseID)
	require.NoError(t, err)
	require.Len(t, result, 1) // Withheld because it's the same owner but a different lease
	require.Equal(t, result[0], "meow.com")

	s.cancel()

	select {
	case <-s.service.lc.Done():
	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestReserveAndReleaseDomain(t *testing.T) {
	s := makeHostnameScaffold(t, []string{"foobar.com", ".bobsdefi.com"})

	leaseID := testutil.LeaseID(t)
	result, err := s.service.ReserveHostnames(s.ctx, []string{"meow.com", "kittens.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	secondLeaseID := testutil.LeaseID(t)
	result, err = s.service.ReserveHostnames(s.ctx, []string{"KITTENS.com"}, secondLeaseID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHostnameNotAllowed))


	err = s.service.ReleaseHostnames(leaseID)
	require.NoError(t, err)

	result, err = s.service.ReserveHostnames(s.ctx, []string{"KITTENS.com"}, secondLeaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	s.cancel()

	select {
	case <-s.service.lc.Done():
	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}
