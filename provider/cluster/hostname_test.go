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

func makeHostnameScaffold(blockedHostnames []string) *scaffold {
	ctx, cancel := context.WithCancel(context.Background())
	v := &scaffold{
		service: newHostnameService(ctx, Config{BlockedHostnames: blockedHostnames}),
		ctx:     ctx,
		cancel:  cancel,
	}

	return v
}

const testWait = time.Second * time.Duration(5)

func TestBlockedHostname(t *testing.T) {
	s := makeHostnameScaffold([]string{"foobar.com", "bobsdefi.com"})

	did := testutil.DeploymentID(t)
	responseCh := s.service.CanReserveHostnames([]string{"foobar.com", "other.org"}, did)
	select {
	case err := <-responseCh:
		require.Error(t, err)
		require.True(t, errors.Is(err, errHostnameNotAllowed))
		require.Regexp(t, "^.*blocked by this provider.*$", err.Error())
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}
	s.cancel()

	select {
	case <-s.service.lc.Done():

	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestBlockedDomain(t *testing.T) {
	s := makeHostnameScaffold([]string{"foobar.com", ".bobsdefi.com"})

	did := testutil.DeploymentID(t)
	responseCh := s.service.CanReserveHostnames([]string{"accounts.bobsdefi.com"}, did)
	select {
	case err := <-responseCh:
		require.Error(t, err)
		require.True(t, errors.Is(err, errHostnameNotAllowed))
		require.Regexp(t, "^.*blocked by this provider.*$", err.Error())
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}
	s.cancel()

	select {
	case <-s.service.lc.Done():

	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func TestReserveMoreHostnamesSameDeployment(t *testing.T) {
	s := makeHostnameScaffold([]string{"foobar.com", ".bobsdefi.com"})

	did := testutil.DeploymentID(t)
	responseCh := s.service.ReserveHostnames([]string{"meow.com", "kittens.com"}, did)
	select {
	case err := <-responseCh:
		require.NoError(t, err)
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}

	responseCh = s.service.ReserveHostnames([]string{"kittens.com", "meow.com", "cats.com"}, did)
	select {
	case err := <-responseCh:
		require.NoError(t, err)
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}

}

func TestReserveAndReleaseDomain(t *testing.T) {
	s := makeHostnameScaffold([]string{"foobar.com", ".bobsdefi.com"})

	did := testutil.DeploymentID(t)
	responseCh := s.service.ReserveHostnames([]string{"meow.com", "kittens.com"}, did)
	select {
	case err := <-responseCh:
		require.NoError(t, err)
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}

	secondDid := testutil.DeploymentID(t)
	responseCh = s.service.ReserveHostnames([]string{"KITTENS.com"}, secondDid)
	select {
	case err := <-responseCh:
		require.Error(t, err)
		require.True(t, errors.Is(err, errHostnameNotAllowed))
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}

	s.service.ReleaseHostnames([]string{"meow.com", "kittens.com"})

	responseCh = s.service.ReserveHostnames([]string{"kittens.com"}, secondDid)
	select {
	case err := <-responseCh:
		require.NoError(t, err)
	case <-time.After(testWait):
		t.Fatal("test timed out waiting on response from service")
	}

	s.cancel()

	select {
	case <-s.service.lc.Done():

	case <-time.After(testWait):
		t.Fatal("timed out waiting for service shutdown")
	}
}
