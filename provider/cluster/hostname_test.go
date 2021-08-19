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

func makeHostnameScaffold(t *testing.T, blockedHostnames []string) *scaffold {
	ctx, cancel := context.WithCancel(context.Background())
	svc, err := newHostnameService(ctx, Config{BlockedHostnames: blockedHostnames}, nil)
	require.NoError(t, err)

	v := &scaffold{
		service: svc,
		ctx:     ctx,
		cancel:  cancel,
	}

	return v
}

const testWait = time.Second * time.Duration(5)

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
	// TODO - tie context used in this test to a timeout
	s := makeHostnameScaffold(t, []string{"foobar.com", ".bobsdefi.com"})

	leaseID := testutil.LeaseID(t)
	result, err := s.service.ReserveHostnames(context.Background(), []string{"meow.com", "kittens.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	result, err = s.service.ReserveHostnames(context.Background(), []string{"kittens.com", "meow.com", "cats.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 1)
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
	result, err := s.service.ReserveHostnames(context.Background(), []string{"meow.com", "kittens.com"}, leaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	secondLeaseID := testutil.LeaseID(t)
	result, err = s.service.ReserveHostnames(context.Background(), []string{"KITTENS.com"}, secondLeaseID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHostnameNotAllowed))


	s.service.ReleaseHostnames(leaseID)

	result, err = s.service.ReserveHostnames(context.Background(), []string{"KITTENS.com"}, secondLeaseID)
	require.NoError(t, err)
	require.Len(t, result, 0)

	s.cancel()

	select {
	case <-s.service.lc.Done():
	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}
