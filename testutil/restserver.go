package testutil

// import (
// 	"context"
// 	"crypto/tls"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"
//
// 	akashclient "github.com/ovrclk/akash/client"
// 	gwutils "github.com/ovrclk/akash/provider/gateway/utils"
// )
//
// func NewServer(t testing.TB, qclient akashclient.QueryClient, handler http.Handler, certs []tls.Certificate) *httptest.Server {
// 	t.Helper()
//
// 	ts := httptest.NewUnstartedServer(handler)
//
// 	var err error
// 	ts.TLS, err = gwutils.NewServerTLSConfig(context.Background(), certs, qclient)
// 	if err != nil {
// 		t.Fatal(err.Error())
// 	}
//
// 	ts.StartTLS()
//
// 	return ts
// }
