package builder

import (
	"github.com/tendermint/tendermint/libs/log"

	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
)

type StorageClassState interface {
	Update(*akashv1.StorageClassState) (*akashv1.StorageClassState, error)
}

// manifest composes the k8s akashv1.Manifest type from LeaseID and
// manifest.Group data.
type storageClassState struct {
}

var _ StorageClassState = (*storageClassState)(nil)

func BuildStorageClassState(log log.Logger) StorageClassState {
	return &storageClassState{}
}

func (b *storageClassState) Update(obj *akashv1.StorageClassState) (*akashv1.StorageClassState, error) {
	// m, err := akashv1.NewStorageClassState(b.Name(), obj.Spec.Capacity, obj.Spec.Available)
	// if err != nil {
	// 	return nil, err
	// }

	return obj, nil
}
