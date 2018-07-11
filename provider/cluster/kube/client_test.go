package kube

import (
	"os"
	"strings"
	"testing"

	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/libs/log"
)

func kubeClient(t *testing.T) Client {
	client, err := NewClient(log.NewTMLogger(os.Stdout), "host", strings.ToLower(t.Name()))
	assert.NoError(t, err)
	return client
}

func leaseID(t *testing.T) types.LeaseID {
	return types.LeaseID{
		Deployment: []byte(t.Name()),
		Group:      0,
		Order:      0,
		Provider:   []byte(t.Name()),
	}
}
