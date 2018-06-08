package state

import (
	"sort"
	"sync"
)

type State interface {
	Get(key []byte) []byte
	Set(key, val []byte)
	Remove(key []byte)
	GetRange([]byte, []byte, int) ([][]byte, [][]byte, error)
	Version() int64

	Account() AccountAdapter
	Deployment() DeploymentAdapter
	Provider() ProviderAdapter
	Order() OrderAdapter
	DeploymentGroup() DeploymentGroupAdapter
	Fulfillment() FulfillmentAdapter
	Lease() LeaseAdapter
}

type CacheState interface {
	State
	Write() error
}

type cacheValue struct {
	value   []byte
	dirty   bool
	removed bool
}

type cache struct {
	cache map[string]cacheValue
	mtx   sync.Mutex
	db    DB
}

func NewCache(db DB) CacheState {
	return &cache{
		cache: make(map[string]cacheValue),
		db:    db,
	}
}

// Get a value from cache or store
func (c *cache) Get(key []byte) []byte {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	k := string(key)
	if v, ok := c.cache[k]; !ok {
		val := c.db.Get(key)
		if len(val) > 0 {
			c.cache[k] = cacheValue{val, false, false}
			return val
		}
		return nil
	} else {
		return v.value
	}
}

// Set value in cache
func (c *cache) Set(key, val []byte) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	k := string(key)
	c.cache[k] = cacheValue{val, true, false}
}

// Set value in cache as removed
func (c *cache) Remove(key []byte) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	k := string(key)
	c.cache[k] = cacheValue{nil, true, true}
}

// Get values from cache and store. Merge results
func (c *cache) GetRange(start, end []byte, limit int) ([][]byte, [][]byte, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	ctr := 0
	s, e := string(start), string(end)
	keys, values := [][]byte{}, [][]byte{}
	for k, v := range c.cache {
		// start --- key --- end
		if s <= k && e >= k {
			// key is in range
			keys = append(keys, []byte(k))
			values = append(values, v.value)
			ctr++
			if ctr >= limit {
				return keys, values, nil
			}
		}
	}
	dbkeys, dbvalues, err := c.db.GetRange(start, end, limit-len(keys))
	if err != nil {
		return nil, nil, err
	}
	for i, k := range dbkeys {
		skey := string(k)
		if _, ok := c.cache[skey]; !ok {
			keys = append(keys, []byte(skey))
			values = append(values, dbvalues[i])
		}
	}
	return keys, values, nil
}

func (c *cache) Write() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	keys := make([]string, 0, len(c.cache))
	for k := range c.cache {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.cache[k]
		if v.removed {
			c.db.Remove([]byte(k))
		} else if v.dirty {
			c.db.Set([]byte(k), v.value)
		}
	}
	c.cache = make(map[string]cacheValue)
	return nil
}

func (c *cache) Version() int64 {
	return c.db.Version()
}

func (c *cache) Account() AccountAdapter {
	return NewAccountAdapter(c)
}

func (c *cache) Deployment() DeploymentAdapter {
	return NewDeploymentAdapter(c)
}

func (c *cache) DeploymentGroup() DeploymentGroupAdapter {
	return NewDeploymentGroupAdapter(c)
}

func (c *cache) Provider() ProviderAdapter {
	return NewProviderAdapter(c)
}

func (c *cache) Order() OrderAdapter {
	return NewOrderAdapter(c)
}

func (c *cache) Fulfillment() FulfillmentAdapter {
	return NewFulfillmentAdapter(c)
}

func (c *cache) Lease() LeaseAdapter {
	return NewLeaseAdapter(c)
}
