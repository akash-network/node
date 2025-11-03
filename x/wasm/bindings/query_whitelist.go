package bindings

import (
	"fmt"
	"sync"

	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	"github.com/cosmos/gogoproto/proto"
)

// queryResponsePools keeps whitelist and its deterministic
// response binding for stargate queries.
// CONTRACT: since results of queries go into blocks, queries being added here should always be
// deterministic or can cause non-determinism in the state machine.
//
// The query is multi-threaded so we're using a sync.Pool
// to manage the allocation and de-allocation of newly created
// pb objects.
var queryResponsePools = make(map[string]*sync.Pool)

// getWhitelistedQuery returns the whitelisted query at the provided path.
// If the query does not exist, or it was setup wrong by the chain, this returns an error.
// CONTRACT: must call returnStargateResponseToPool in order to avoid pointless allocs.
func getWhitelistedQuery(queryPath string) (proto.Message, error) {
	protoResponseAny, isWhitelisted := queryResponsePools[queryPath]
	if !isWhitelisted {
		return nil, wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("'%s' path is not allowed from the contract", queryPath)}
	}
	protoMarshaler, ok := protoResponseAny.Get().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to assert type to proto.Messager")
	}
	return protoMarshaler, nil
}

// returnStargateResponseToPool returns the provided protoMarshaler to the appropriate pool based on it's query path.
func returnQueryResponseToPool(queryPath string, pb proto.Message) {
	queryResponsePools[queryPath].Put(pb)
}
