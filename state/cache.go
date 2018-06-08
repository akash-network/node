package state

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

type CacheState interface {
	Get(key []byte) []byte
	Set(key, val []byte)
	Remove(key []byte)
	GetRange([]byte, []byte) ([][]byte, [][]byte, error)
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
	k := string(key)
	if v, ok := c.cache[k]; !ok {
		val := c.db.Get(key)
		c.cache[k] = cacheValue{val, false, false}
		return val
	} else {
		return v.value
	}
}

// Set value in cache
func (c *cache) Set(key, val []byte) {
	k := string(key)
	c.cache[k] = cacheValue{val, true, false}
}

// Set value in cache as removed
func (c *cache) Remove(key []byte) {
	k := string(key)
	c.cache[k] = cacheValue{nil, true, true}
}

// Get values from cache and store. Merge results
func (c *cache) GetRange(start, end []byte) ([][]byte, [][]byte, error) {
	s, e := string(start), string(end)
	keys, values := [][]byte{}, [][]byte{}
	for k, v := range c.cache {
		if strings.Compare(s, k) == -1 && strings.Compare(e, k) == 1 {
			// key is in range
			keys = append(keys, []byte(k))
			values = append(values, v.value)
		}
	}
	dbkeys, dbvalues, _, err := c.db.GetRangeWithProof(start, end, MaxRangeLimit)
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
	keys := make([]string, 0, len(c.cache))
	for k := range c.cache {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.cache[k]
		if v.removed {
			_, removed := c.db.Remove([]byte(k))
			if !removed {
				errors.New("remove should have removed but did not remove.")
			}
		} else if v.dirty {
			c.db.Set([]byte(k), v.value)
		}
	}
	return nil
}
