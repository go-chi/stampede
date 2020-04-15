package stampede

import (
	"sync"
	"time"

	"github.com/goware/stampede/singleflight"
)

/* Prevents cache stampede https://en.wikipedia.org/wiki/Cache_stampede by only running a single data fetch operation per expired / missing key
 * regardless of number of requests to that key
 */

type value struct {
	v interface{}

	bestBefore time.Time // cache entry freshness cutoff
	expiry     time.Time // cache entry time to live cutoff
}

func (v *value) IsFresh() bool {
	if v == nil {
		return false
	}

	return v.bestBefore.After(time.Now())
}

func (v *value) IsExpired() bool {
	if v == nil {
		return true
	}

	return v.expiry.Before(time.Now())
}

func (v *value) Val() interface{} {
	return v.v
}

type Cache struct {
	values map[string]*value

	freshFor time.Duration
	ttl      time.Duration

	mu      sync.RWMutex
	callGrp singleflight.Group
}

func NewCache(freshFor, ttl time.Duration) *Cache {
	return &Cache{
		freshFor: freshFor,
		ttl:      ttl,
		values:   make(map[string]*value),
	}
}

func (c *Cache) Get(key string, fn func() (interface{}, error), freshOnly bool) (interface{}, error) {
	c.mu.RLock()
	val, _ := c.values[key]
	c.mu.RUnlock()

	// value exists and is fresh - just return
	if val.IsFresh() {
		return val.Val(), nil
	}

	// value exists and is stale, and we're OK with serving it stale while updaing in the background
	if !freshOnly && !val.IsExpired() {
		go c.Set(key, fn)

		return val.Val(), nil
	}

	// value doesn't exist or is expired, or is stale and we need it fresh - sync update
	v, err, _ := c.Set(key, fn)
	return v, err
}

func (c *Cache) GetFresh(key string, fn func() (interface{}, error)) (interface{}, error) {
	return c.Get(key, fn, true)
}

func (c *Cache) Set(key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	return c.callGrp.Do(key, c.set(key, fn))
}

func (c *Cache) set(key string, fn func() (interface{}, error)) func() (interface{}, error) {
	return func() (interface{}, error) {
		val, err := fn()
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.values[key] = &value{
			v:          val,
			expiry:     time.Now().Add(c.ttl),
			bestBefore: time.Now().Add(c.freshFor),
		}
		c.mu.Unlock()

		return val, nil
	}
}
