package kube

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/libs/log"
)

func kubeClient(t *testing.T) Client {
	client, err := NewClient(log.NewTMLogger(os.Stdout), "host", strings.ToLower(t.Name()))
	assert.NoError(t, err)
	return client
}

// func leaseID(t *testing.T) mtypes.LeaseID {
// 	return mtypes.LeaseID{}
// }
