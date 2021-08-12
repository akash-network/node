package bidengine

import (
	"errors"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clientmocks "github.com/ovrclk/akash/client/mocks"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	akashtypes "github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/x/audit/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type providerAttributesTestScaffold struct {
	service      *providerAttrSignatureService
	provider     *ptypes.Provider
	s            session.Session
	bus          pubsub.Bus
	client       *clientmocks.Client
	queryClient  *clientmocks.QueryClient
	auditorAddr  sdk.AccAddress
	providerAddr sdk.AccAddress
}

func setupProviderAttributesTestScaffold(t *testing.T, ttl time.Duration, clientFactory func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient) *providerAttributesTestScaffold {
	retval := &providerAttributesTestScaffold{
		auditorAddr:  testutil.AccAddress(t),
		providerAddr: testutil.AccAddress(t),
	}
	retval.provider = &ptypes.Provider{
		Owner:      retval.providerAddr.String(),
		HostURI:    "http://foo.localhost:8443",
		Attributes: nil,
		Info:       ptypes.ProviderInfo{},
	}

	retval.client = &clientmocks.Client{}

	retval.queryClient = clientFactory(retval)
	retval.client.On("Query").Return(retval.queryClient)
	retval.s = session.New(testutil.Logger(t), retval.client, retval.provider)
	retval.bus = pubsub.NewBus()
	var err error
	retval.service, err = newProviderAttrSignatureServiceInternal(retval.s, retval.bus, ttl)
	require.NoError(t, err)

	return retval
}

func (scaffold *providerAttributesTestScaffold) stop(t *testing.T) {
	scaffold.service.lc.Shutdown(nil)

	select {
	case <-scaffold.service.lc.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for service to stop")
	}
	scaffold.bus.Close()
}

var errWithExpectedText = errors.New("invalid provider: address not found")

func TestProvAttrCachesValue(t *testing.T) {
	scaffold := setupProviderAttributesTestScaffold(t, 1*time.Hour, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		req := &atypes.QueryProviderAuditorRequest{
			Owner:   scaffold.providerAddr.String(),
			Auditor: scaffold.auditorAddr.String(),
		}
		queryClient := &clientmocks.QueryClient{}
		response := &atypes.QueryProvidersResponse{
			Providers: atypes.Providers{
				atypes.Provider{
					Owner: scaffold.providerAddr.String(),
					Attributes: akashtypes.Attributes{
						akashtypes.Attribute{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
			},
			Pagination: nil,
		}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, req).Return(response, nil)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	attrs, err = scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	scaffold.stop(t)

	// Should have 2 calls
	require.Len(t, scaffold.queryClient.Calls, 2)
}

func TestProvAttrReturnsEmpty(t *testing.T) {
	scaffold := setupProviderAttributesTestScaffold(t, 1*time.Hour, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		req := &atypes.QueryProviderAuditorRequest{
			Owner:   scaffold.providerAddr.String(),
			Auditor: scaffold.auditorAddr.String(),
		}
		queryClient := &clientmocks.QueryClient{}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, req).Return(nil, errWithExpectedText)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 0) // Nothing is returned

	_, err = scaffold.service.GetAttributes()
	require.NoError(t, err)

	scaffold.stop(t)

	// Should have just 1 call
	require.Len(t, scaffold.queryClient.Calls, 2)
}

func TestProvAttrObeysTTL(t *testing.T) {
	const ttl = 100 * time.Millisecond
	scaffold := setupProviderAttributesTestScaffold(t, ttl, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		req := &atypes.QueryProviderAuditorRequest{
			Owner:   scaffold.providerAddr.String(),
			Auditor: scaffold.auditorAddr.String(),
		}
		queryClient := &clientmocks.QueryClient{}
		response := &atypes.QueryProvidersResponse{
			Providers: atypes.Providers{
				atypes.Provider{
					Owner: scaffold.providerAddr.String(),
					Attributes: akashtypes.Attributes{
						akashtypes.Attribute{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
			},
			Pagination: nil,
		}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, req).Return(response, nil)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	time.Sleep(2 * ttl)

	attrs, err = scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	scaffold.stop(t)

	// Should have just 1 call
	require.Len(t, scaffold.queryClient.Calls, 3)
}

func TestProvAttrTrimsCache(t *testing.T) {
	const ttl = 1 * time.Hour
	scaffold := setupProviderAttributesTestScaffold(t, ttl, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		queryClient := &clientmocks.QueryClient{}
		attrs := make([]akashtypes.Attribute, 1001)
		for i := range attrs {
			attrs[i] = akashtypes.Attribute{
				Key:   "foo",
				Value: "bar",
			}
		}
		response := &atypes.QueryProvidersResponse{
			Providers: atypes.Providers{
				atypes.Provider{
					Owner:      scaffold.providerAddr.String(),
					Attributes: attrs,
				},
			},
			Pagination: nil,
		}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, mock.Anything).Return(response, nil)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.NotNil(t, attrs)

	addrs := make([]sdk.AccAddress, 1)
	for i := 0; i != 51; i++ {
		addr := testutil.AccAddress(t)
		addrs = append(addrs, addr)
		attrs, err := scaffold.service.GetAuditorAttributeSignatures(addr.String())
		require.NoError(t, err)
		require.NotNil(t, attrs)
	}

	for _, addr := range addrs {
		attrs, err := scaffold.service.GetAuditorAttributeSignatures(addr.String())
		require.NoError(t, err)
		require.NotNil(t, attrs)
	}

	scaffold.stop(t)

	// Should have more calls then addresses, since things get pushed out of the cache
	require.Greater(t, len(scaffold.queryClient.Calls), len(addrs))
}

var errForTest = errors.New("an error used only for test")

func TestProvAttrReturnsErrors(t *testing.T) {
	const ttl = 1 * time.Hour
	scaffold := setupProviderAttributesTestScaffold(t, ttl, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		queryClient := &clientmocks.QueryClient{}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, mock.Anything).Return(nil, errForTest)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.ErrorIs(t, err, errForTest)
	require.Nil(t, attrs)

	scaffold.stop(t)
}

func TestProvAttrClearsCache(t *testing.T) {
	const ttl = 1 * time.Hour
	scaffold := setupProviderAttributesTestScaffold(t, ttl, func(scaffold *providerAttributesTestScaffold) *clientmocks.QueryClient {
		req := &atypes.QueryProviderAuditorRequest{
			Owner:   scaffold.providerAddr.String(),
			Auditor: scaffold.auditorAddr.String(),
		}
		queryClient := &clientmocks.QueryClient{}
		response := &atypes.QueryProvidersResponse{
			Providers: atypes.Providers{
				atypes.Provider{
					Owner: scaffold.providerAddr.String(),
					Attributes: akashtypes.Attributes{
						akashtypes.Attribute{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
			},
			Pagination: nil,
		}
		queryClient.On("ProviderAuditorAttributes", mock.Anything, req).Return(response, nil)

		attrReq := &ptypes.QueryProviderRequest{
			Owner: scaffold.provider.Owner,
		}

		attrResp := &ptypes.QueryProviderResponse{
			Provider: ptypes.Provider{
				Owner:   scaffold.providerAddr.String(),
				HostURI: "",
				Attributes: akashtypes.Attributes{
					akashtypes.Attribute{
						Key:   "foo",
						Value: "bar",
					},
				},
			},
		}
		queryClient.On("Provider", mock.Anything, attrReq).Return(attrResp, nil)

		return queryClient
	})

	attrs, err := scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	err = scaffold.bus.Publish(atypes.EventTrustedAuditorCreated{
		Owner:   scaffold.providerAddr,
		Auditor: scaffold.auditorAddr,
	})
	require.NoError(t, err)
	time.Sleep(5 * time.Second) // Allow event to be received

	attrs, err = scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	err = scaffold.bus.Publish(atypes.EventTrustedAuditorDeleted{
		Owner:   scaffold.providerAddr,
		Auditor: scaffold.auditorAddr,
	})
	require.NoError(t, err)
	time.Sleep(5 * time.Second) // Allow event to be received

	attrs, err = scaffold.service.GetAuditorAttributeSignatures(scaffold.auditorAddr.String())
	require.NoError(t, err)
	require.Len(t, attrs, 1)

	scaffold.stop(t)

	// Should have 3 calls
	require.Len(t, scaffold.queryClient.Calls, 4)
}
