package cluster

import (
	"testing"

	"github.com/ovrclk/akash/client"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

/* Source: https://git.sr.ht/~samwhited/testlog/tree/b1b3e8e82fd6990e91ce9d0fbcbe69ac2d9b1f98/testlog.go
// New returns a new logger that logs to the provided testing.T.
func New(t testing.TB) *log.Logger {
	t.Helper()
	return log.New(testWriter{TB: t}, t.Name()+" ", log.LstdFlags|log.Lshortfile|log.LUTC)
}
*/

type testWriter struct {
	testing.TB
}

func (tw testWriter) Write(p []byte) (int, error) {
	tw.Helper()
	tw.Logf("%s", p)
	return len(p), nil
}

var (
	stateDeployTests = []struct {
		lease mtypes.LeaseID
	}{}
)

/*
type Client interface {
	Query() QueryClient
	Tx() TxClient
}
*/
type cosmosClient struct{}

func (cc *cosmosClient) Query() client.QueryClient {
	return nil
}
func (cc *cosmosClient) Tx() client.TxClient {
	return nil
}

func TestStateDeploy(t *testing.T) {
	/* TODO: Improve utilities around testing the CosmosSDK so tests
	can be run against the State machine.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	//tw := new(testWriter)
	l := log.NewNopLogger()
	lease := mtypes.LeaseID{}
	mg := &manifest.Group{}

	nc := &nullClient{
		leases: make(map[string]*manifest.Group),
	} //*blockchain client*
	cc := &cosmosClient{}

	bus := pubsub.NewBus()
	sess := session.New(l, cc, &query.Provider{})
	// TODO: provider := Provider{...owner host attributes}

	s, err := NewService(ctx, sess, bus, nc)
	if err != nil {
		t.Fatal(err)
	}
	//NewService(ctx context.Context, session session.Session, bus pubsub.Bus, client Client)
	sStruct, ok := s.(*service)
	if !ok {
		t.Fatal("service failed type assertion")
	}

	// completely empty initialization
	ds, state := newTestDeployState(ctx, sStruct, lease, mg)
	t.Logf("%+v state: %+v", ds, state)
	*/
}
