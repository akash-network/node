package sdl

import (
	"bytes"
	"fmt"
	types "github.com/ovrclk/akash/types/v1beta2"
	"github.com/stretchr/testify/require"
	"testing"
)

func findFirstIPEndpoint(t *testing.T, endpoints []types.Endpoint) types.Endpoint {
	for _, endpoint := range endpoints {
		if endpoint.Kind == types.Endpoint_LEASED_IP {
			return endpoint
		}
	}

	t.Fatal("did not find any IP endpoints")
	return types.Endpoint{}
}

func TestV2Parse_IP(t *testing.T) {
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2-ip-endpoint.yaml")
	require.NoError(t, err)
	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)

	require.Len(t, groups, 1)
	group := groups[0]

	resources := group.GetResources()
	require.Len(t, resources, 1)
	resource := resources[0]
	endpoints := resource.Resources.Endpoints
	require.Len(t, endpoints, 2)

	var ipEndpoint types.Endpoint
	for _, endpoint := range endpoints {
		if endpoint.Kind == types.Endpoint_LEASED_IP {
			ipEndpoint = endpoint
		}
	}

	require.Equal(t, ipEndpoint.Kind, types.Endpoint_LEASED_IP)
	require.Greater(t, ipEndpoint.SequenceNumber, uint32(0))

	mani, err := sdl1.Manifest()
	require.NoError(t, err)
	maniGroups := mani.GetGroups()
	require.Len(t, maniGroups, 1)
	maniGroup := maniGroups[0]
	services := maniGroup.Services
	require.Len(t, services, 1)

	service := services[0]
	exposes := service.Expose
	require.Len(t, exposes, 1)

	expose := exposes[0]

	require.True(t, expose.Global)
	require.Equal(t, expose.IP, "meow")
	require.Greater(t, expose.EndpointSequenceNumber, uint32(0))
}

func TestV2Parse_SharedIP(t *testing.T) {
	// Read a file with 1 group having 1 endpoint shared amongst containers
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2-shared-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1)

	group := groups[0]

	resources := group.GetResources()
	require.Len(t, resources, 2)

	resource := resources[0]
	ipEndpoint := findFirstIPEndpoint(t, resource.Resources.Endpoints)
	require.Greater(t, ipEndpoint.SequenceNumber, uint32(0))

	resource = resources[1]
	ipEndpoint = findFirstIPEndpoint(t, resource.Resources.Endpoints)
	require.Greater(t, ipEndpoint.SequenceNumber, uint32(0))

	mani, err := sdl1.Manifest()
	require.NoError(t, err)

	maniGroups := mani.GetGroups()
	require.Len(t, maniGroups, 1)
	maniGroup := maniGroups[0]

	services := maniGroup.Services
	require.Len(t, services, 2)
	serviceA := services[0]

	serviceIPEndpoint := findFirstIPEndpoint(t, serviceA.Resources.Endpoints)
	require.Equal(t, serviceIPEndpoint.SequenceNumber, ipEndpoint.SequenceNumber)

	serviceB := services[1]
	serviceIPEndpoint = findFirstIPEndpoint(t, serviceB.Resources.Endpoints)
	require.Equal(t, serviceIPEndpoint.SequenceNumber, ipEndpoint.SequenceNumber)
}

func TestV2Parse_MultipleIP(t *testing.T) {
	// Read a file with 1 group having two endpoints
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2-multi-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1)

	group := groups[0]

	resources := group.GetResources()
	require.Len(t, resources, 2)

	mani, err := sdl1.Manifest()
	require.NoError(t, err)
	_ = mani
}

func TestV2Parse_MultipleGroupsIP(t *testing.T) {
	// Read a file with two groups, each one having an IP endpoint that is distinct
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2-multi-groups-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 2)

	resources := groups[0].GetResources()
	require.Len(t, resources, 1)

	resource := resources[0]
	require.Len(t, resource.Resources.Endpoints, 2)
	ipEndpointFirstGroup := findFirstIPEndpoint(t, resource.Resources.Endpoints)
	require.Greater(t, ipEndpointFirstGroup.SequenceNumber, uint32(0))

	resources = groups[1].GetResources()
	require.Len(t, resources, 1)

	resource = resources[0]
	require.Len(t, resource.Resources.Endpoints, 2)
	ipEndpointSecondGroup := findFirstIPEndpoint(t, resource.Resources.Endpoints)
	require.Greater(t, ipEndpointSecondGroup.SequenceNumber, uint32(0))
	require.NotEqual(t, ipEndpointFirstGroup.SequenceNumber, ipEndpointSecondGroup.SequenceNumber)

	mani, err := sdl1.Manifest()
	require.NoError(t, err)
	maniGroups := mani.GetGroups()
	require.Len(t, maniGroups, 2)

	maniGroup := maniGroups[0]
	resources = maniGroup.GetResources()
	require.Len(t, resources, 1)
	resource = resources[0]
	require.Equal(t, findFirstIPEndpoint(t, resource.Resources.Endpoints).SequenceNumber, ipEndpointFirstGroup.SequenceNumber)

	maniGroup = maniGroups[1]
	resources = maniGroup.GetResources()
	require.Len(t, resources, 1)
	resource = resources[0]
	require.Equal(t, findFirstIPEndpoint(t, resource.Resources.Endpoints).SequenceNumber, ipEndpointSecondGroup.SequenceNumber)

}

func TestV2Parse_IPEndpointNaming(t *testing.T) {
	makeSDLWithEndpointName := func(name string) []byte {
		const originalSDL = `---
version: "2.0"

services:
  web:
    image: quay.io/ovrclk/demo-app
    expose:
      - port: 80
        to:
          - global: true
            ip: %q
        accept:
          - test.localhost         

profiles:
  compute:
    web:
      resources:
        cpu:
          units: "0.01"
        memory:
          size: "128Mi"
        storage:
          size: "512Mi"

  placement:
    global:
      pricing:
        web:
          denom: uakt
          amount: 10

deployment:
  web:
    global:
      profile: web
      count: 1

endpoints:
  %q:
    kind: ip
`
		buf := &bytes.Buffer{}
		_, err := fmt.Fprintf(buf, originalSDL, name, name)
		require.NoError(t, err)
		return buf.Bytes()
	}

	_, err := Read(makeSDLWithEndpointName("meow72-memes"))
	require.NoError(t, err)

	_, err = Read(makeSDLWithEndpointName("meow72-mem_es"))
	require.NoError(t, err)

	_, err = Read(makeSDLWithEndpointName("!important"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

	_, err = Read(makeSDLWithEndpointName("foo^bar"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

	_, err = Read(makeSDLWithEndpointName("ROAR"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

	_, err = Read(makeSDLWithEndpointName("996"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

	_, err = Read(makeSDLWithEndpointName("_kittens"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

	_, err = Read(makeSDLWithEndpointName("-kittens"))
	require.Error(t, err)
	require.ErrorIs(t, err, errSDLInvalid)
	require.Contains(t, err.Error(), "not a valid name")

}
