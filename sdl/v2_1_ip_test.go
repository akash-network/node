package sdl

import (
	"bytes"
	"fmt"
	"testing"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	"github.com/stretchr/testify/require"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"
)

func TestV2_1_ParseSimpleWithIP(t *testing.T) {
	sdl, err := ReadFile("./_testdata/v2.1-simple-with-ip.yaml")
	require.NoError(t, err)
	require.NotNil(t, sdl)

	groups, err := sdl.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1)
	group := groups[0]
	resources := group.GetResourceUnits()
	require.Len(t, resources, 1)
	resource := resources[0]

	ipEndpoint := findIPEndpoint(t, resource.Resources.Endpoints, 1)

	require.Equal(t, ipEndpoint.Kind, types.Endpoint_LEASED_IP)

	mani, err := sdl.Manifest()
	require.NoError(t, err)
	var exposeIP manifest.ServiceExpose
	for _, expose := range mani[0].Services[0].Expose {
		if len(expose.IP) != 0 {
			exposeIP = expose
			break
		}
	}
	require.NotEmpty(t, exposeIP.IP)
	require.Equal(t, exposeIP.Proto, manifest.UDP)
	require.Equal(t, exposeIP.Port, uint32(12345))
	require.True(t, exposeIP.Global)
}

func TestV2_1_Parse_IP(t *testing.T) {
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2.1-ip-endpoint.yaml")
	require.NoError(t, err)
	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)

	require.Len(t, groups, 1)
	group := groups[0]

	resources := group.GetResourceUnits()
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

func TestV2_1_Parse_SharedIP(t *testing.T) {
	// Read a file with 1 group having 1 endpoint shared amongst containers
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2.1-shared-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1)

	group := groups[0]

	resources := group.GetResourceUnits()
	require.Len(t, resources, 1)

	resource := resources[0]
	ipEndpoint1 := findIPEndpoint(t, resource.Resources.Endpoints, 1)
	require.Greater(t, ipEndpoint1.SequenceNumber, uint32(0))

	ipEndpoint2 := findIPEndpoint(t, resource.Resources.Endpoints, 2)
	require.Greater(t, ipEndpoint2.SequenceNumber, uint32(0))

	mani, err := sdl1.Manifest()
	require.NoError(t, err)

	maniGroups := mani.GetGroups()
	require.Len(t, maniGroups, 1)
	maniGroup := maniGroups[0]

	services := maniGroup.Services
	require.Len(t, services, 2)
	serviceA := services[0]

	serviceIPEndpoint := findIPEndpoint(t, serviceA.Resources.Endpoints, 1)
	require.Equal(t, serviceIPEndpoint.SequenceNumber, ipEndpoint1.SequenceNumber)

	serviceB := services[1]
	serviceIPEndpoint = findIPEndpoint(t, serviceB.Resources.Endpoints, 1)
	require.Equal(t, serviceIPEndpoint.SequenceNumber, ipEndpoint2.SequenceNumber)
}

func TestV2_1_Parse_MultipleIP(t *testing.T) {
	// Read a file with 1 group having two endpoints
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2.1-multi-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1)

	group := groups[0]

	resources := group.GetResourceUnits()
	require.Len(t, resources, 1)

	mani, err := sdl1.Manifest()
	require.NoError(t, err)
	_ = mani
}

func TestV2_1_Parse_MultipleGroupsIP(t *testing.T) {
	// Read a file with two groups, each one having an IP endpoint that is distinct
	sdl1, err := ReadFile("../x/deployment/testdata/deployment-v2.1-multi-groups-ip-endpoint.yaml")
	require.NoError(t, err)

	groups, err := sdl1.DeploymentGroups()
	require.NoError(t, err)
	require.Len(t, groups, 2)

	resources := groups[0].GetResourceUnits()
	require.Len(t, resources, 1)

	resource := resources[0]
	require.Len(t, resource.Resources.Endpoints, 2)
	ipEndpointFirstGroup := findIPEndpoint(t, resource.Resources.Endpoints, 1)
	require.Greater(t, ipEndpointFirstGroup.SequenceNumber, uint32(0))

	resources = groups[1].GetResourceUnits()
	require.Len(t, resources, 1)

	resource = resources[0]
	require.Len(t, resource.Resources.Endpoints, 2)
	ipEndpointSecondGroup := findIPEndpoint(t, resource.Resources.Endpoints, 1)
	require.Greater(t, ipEndpointSecondGroup.SequenceNumber, uint32(0))
	require.NotEqual(t, ipEndpointFirstGroup.SequenceNumber, ipEndpointSecondGroup.SequenceNumber)

	mani, err := sdl1.Manifest()
	require.NoError(t, err)
	maniGroups := mani.GetGroups()
	require.Len(t, maniGroups, 2)

	maniGroup := maniGroups[0]
	mresources := maniGroup.GetResourceUnits()
	require.Len(t, mresources, 1)
	mresource := mresources[0]
	require.Equal(t, findIPEndpoint(t, mresource.Endpoints, 1).SequenceNumber, ipEndpointFirstGroup.SequenceNumber)

	maniGroup = maniGroups[1]
	mresources = maniGroup.GetResourceUnits()
	require.Len(t, mresources, 1)
	mresource = mresources[0]
	require.Equal(t, findIPEndpoint(t, mresource.Endpoints, 1).SequenceNumber, ipEndpointSecondGroup.SequenceNumber)

}

func TestV2_1_Parse_IPEndpointNaming(t *testing.T) {
	makeSDLWithEndpointName := func(name string) []byte {
		const originalSDL = `---
version: "2.1"

services:
  web:
    image: ghcr.io/akash-network/demo-app
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
