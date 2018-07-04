package kube

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/log"
)

func kubeClient(t *testing.T) Client {
	client, err := NewClient(log.NewTMLogger(os.Stdout), strings.ToLower(t.Name()))
	assert.NoError(t, err)
	return client
}

func tearDown(client Client, t *testing.T) {
	err := client.TeardownNamespace(strings.ToLower(t.Name()))
	assert.NoError(t, err)
}

func leaseID(t *testing.T) types.LeaseID {
	return types.LeaseID{
		Deployment: []byte(t.Name()),
		Group:      0,
		Order:      0,
		Provider:   []byte(t.Name()),
	}
}

// Integration
func TestLeaseStatus(t *testing.T) {
	t.SkipNow()
	sdl, err := sdl.ReadFile("../../../_integration/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	client := kubeClient(t)
	leaseID := leaseID(t)

	client.Deploy(leaseID, mani.Groups[0])
	time.Sleep(5 * time.Second)

	status, err := client.LeaseStatus(leaseID)
	assert.NoError(t, err)

	fmt.Println(status)
	assert.Len(t, status.Services, 1)
	assert.Equal(t, status.Services[0].Name, "web")
	assert.Equal(t, status.Services[0].Status, "available replicas: 0/20")

	err = client.TeardownLease(leaseID)
	assert.NoError(t, err)
	tearDown(client, t)
}

// Integration
func TestServiceStatus(t *testing.T) {
	t.SkipNow()
	sdl, err := sdl.ReadFile("../../../_run/kube/deployment.yml")
	require.NoError(t, err)

	mani, err := sdl.Manifest()
	require.NoError(t, err)

	client := kubeClient(t)
	leaseID, err := keys.ParseLeasePath("f82532a61ea1783a47577cbf5044748e3a60805132753f263d2f16d6c0479110/0/0/8224e14f903a2e136a6362527b19f11935197175cb69981940933aa04459a2a9")
	assert.NoError(t, err)

	client.Deploy(leaseID.LeaseID, mani.Groups[0])

	time.Sleep(20 * time.Second)

	status, err := client.ServiceStatus(leaseID.LeaseID, "web")
	assert.NoError(t, err)

	assert.Equal(t, int32(2), status.AvailableReplicas)

	err = client.TeardownLease(leaseID.LeaseID)
	assert.NoError(t, err)
	tearDown(client, t)
}

// Integration
func TestServiceLog(t *testing.T) {
	t.Skip()
	leaseID, err := keys.ParseLeasePath("f82532a61ea1783a47577cbf5044748e3a60805132753f263d2f16d6c0479110/0/0/8224e14f903a2e136a6362527b19f11935197175cb69981940933aa04459a2a9")

	client := kubeClient(t)

	logs, err := client.ServiceLogs(context.TODO(), leaseID.LeaseID, 1000, true)
	assert.NoError(t, err)

	fmt.Println("got streams: ", (len(logs)))

LOOP:
	for {
		for _, log := range logs {
			scanner := bufio.NewScanner(log.Stream)
			scanner.Scan()
			fmt.Println("[" + log.Name + "]" + scanner.Text())
			if scanner.Err() != nil {
				break LOOP
			}
		}
	}

	t.Fail()

}
