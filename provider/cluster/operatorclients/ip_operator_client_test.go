package operatorclients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/cosmos/cosmos-sdk/server"
	ipoptypes "github.com/ovrclk/akash/provider/operator/ipoperator/types"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"

	"testing"
	"time"
)

type fakeIPOperator struct {
	healthStatus uint32
	mux          *http.ServeMux

	ipLeaseStatusResponse atomic.Value
	ipLeaseStatusStatus   uint32

	ipUsageStatus   uint32
	ipUsageResponse atomic.Value
}

func (fio *fakeIPOperator) setHealthStatus(status int) {
	atomic.StoreUint32(&fio.healthStatus, uint32(status))
}

func (fio *fakeIPOperator) setIPLeaseStatusResponse(status int, body []byte) {
	atomic.StoreUint32(&fio.ipLeaseStatusStatus, uint32(status))
	fio.ipLeaseStatusResponse.Store(body)
}

func (fio *fakeIPOperator) setIPUsageResponse(status int, body []byte) {
	atomic.StoreUint32(&fio.ipUsageStatus, uint32(status))
	fio.ipUsageResponse.Store(body)
}

func fakeIPOperatorHandler() *fakeIPOperator {
	fake := &fakeIPOperator{
		healthStatus:        http.StatusServiceUnavailable,
		mux:                 http.NewServeMux(),
		ipLeaseStatusStatus: http.StatusServiceUnavailable,
		ipUsageStatus:       http.StatusServiceUnavailable,
	}
	fake.ipLeaseStatusResponse.Store([]byte{})
	fake.ipUsageResponse.Store([]byte{})

	fake.mux.HandleFunc("/health",
		func(rw http.ResponseWriter, req *http.Request) {
			status := atomic.LoadUint32(&fake.healthStatus)
			rw.WriteHeader(int(status))
		})

	fake.mux.HandleFunc("/ip-lease-status/", func(rw http.ResponseWriter, req *http.Request) {
		status := atomic.LoadUint32(&fake.ipLeaseStatusStatus)
		rw.WriteHeader(int(status))

		body := fake.ipLeaseStatusResponse.Load().([]byte)
		_, _ = io.Copy(rw, bytes.NewReader(body))
	})

	fake.mux.HandleFunc("/usage", func(rw http.ResponseWriter, req *http.Request) {
		status := atomic.LoadUint32(&fake.ipUsageStatus)
		rw.WriteHeader(int(status))

		body := fake.ipUsageResponse.Load().([]byte)
		_, _ = io.Copy(rw, bytes.NewReader(body))
	})

	return fake
}

func TestIPOperatorClient(t *testing.T) {
	_, port, err := server.FreeTCPAddr()
	require.NoError(t, err)

	fake := fakeIPOperatorHandler()
	fakeServer := &http.Server{
		Addr:         "localhost:" + port,
		Handler:      fake.mux,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	go func() {
		err := fakeServer.ListenAndServe()
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	defer func() {
		_ = fakeServer.Close()
	}()

	// Wait for http server to start
	time.Sleep(time.Second)

	logger := testutil.Logger(t)
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)

	srv := net.SRV{
		Target:   "localhost",
		Port:     uint16(portNumber),
		Priority: 0,
		Weight:   0,
	}

	ipop, err := NewIPOperatorClient(logger, nil, &srv)
	require.NoError(t, err)
	require.NotNil(t, ipop)
	defer ipop.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Description should have a value
	require.NotEmpty(t, ipop.String())

	// Should not be ready
	require.ErrorIs(t, ipop.Check(ctx), errNotAlive)

	// Set it to function now
	fake.setHealthStatus(http.StatusOK)

	// Should be OK now
	require.NoError(t, ipop.Check(ctx))

	// Get the status of an order ID that does not exist
	status, err := ipop.GetIPAddressStatus(ctx, testutil.OrderID(t))
	require.ErrorIs(t, err, errIPOperatorRemote)
	require.Nil(t, status)

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	sample := ipoptypes.LeaseIPStatus{
		Port:         1,
		ExternalPort: 2,
		ServiceName:  "a",
		IP:           "b",
		Protocol:     "c",
	}
	require.NoError(t, enc.Encode([]ipoptypes.LeaseIPStatus{
		sample,
	}))
	fake.setIPLeaseStatusResponse(http.StatusOK, body.Bytes())

	status, err = ipop.GetIPAddressStatus(ctx, testutil.OrderID(t))
	require.NoError(t, err)
	require.Len(t, status, 1)
	require.Equal(t, sample, status[0])

	usage, err := ipop.GetIPAddressUsage(ctx)
	require.ErrorIs(t, err, errIPOperatorRemote)
	require.Zero(t, usage)

	body = &bytes.Buffer{}
	enc = json.NewEncoder(body)
	require.NoError(t, enc.Encode(
		ipoptypes.IPAddressUsage{
			Available: 13,
			InUse:     14,
		}))
	fake.setIPUsageResponse(http.StatusOK, body.Bytes())
	usage, err = ipop.GetIPAddressUsage(ctx)
	require.NoError(t, err)
	require.Equal(t, ipoptypes.IPAddressUsage{
		Available: 13,
		InUse:     14,
	}, usage)

}
