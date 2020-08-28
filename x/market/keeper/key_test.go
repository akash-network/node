package keeper

// import (
// 	"testing"

// 	"github.com/ovrclk/akash/testutil"
// 	"github.com/stretchr/testify/assert"
// )

// func TestActiveLeaseKeys(t *testing.T) {
// 	lease := testutil.LeaseID(t)
// 	key := leaseKey(lease)
// 	activeKey := leaseKeyActive(lease)
// 	assert.NotEqual(t, key, activeKey)

// 	t.Run("assert converted active lease key matches data key", func(t *testing.T) {
// 		convertedActiveKey, err := convertLeaseActiveKey(activeKey)
// 		assert.NoError(t, err)
// 		assert.Equal(t, convertedActiveKey, key)
// 	})
// }

// func TestOrdersOpenKeys(t *testing.T) {
// 	oid := testutil.OrderID(t)
// 	key := orderKey(oid)
// 	openKey := orderOpenKey(oid)
// 	assert.NotEqual(t, key, openKey)

// 	t.Run("assert converted order open key matches normal order key", func(t *testing.T) {
// 		convertedKey, err := convertOrderOpenKey(openKey)
// 		assert.NoError(t, err)
// 		assert.Equal(t, convertedKey, key)
// 	})
// }
