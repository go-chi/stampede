package stampede

import (
	"context"
	"sync"
	"time"

	"github.com/goware/singleflight"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/zeebo/xxh3"
)

// Prevents cache stampede https://en.wikipedia.org/wiki/Cache_stampede by only running a
// single data fetch operation per expired / missing key regardless of number of requests to that key.

func NewCache(size int, freshFor, ttl time.Duration) *Cache[any, any] {
	return NewCacheKV[any, any](size, freshFor, ttl)
}

func NewCacheKV[K comparable, V any](size int, freshFor, ttl time.Duration) *Cache[K, V] {
	values, _ := lru.New[K, value[V]](size)
	return &Cache[K, V]{
		freshFor: freshFor,
		ttl:      ttl,
		values:   values,
	}
}

type Cache[K comparable, V any] struct {
	values *lru.Cache[K, value[V]]

	freshFor time.Duration
	ttl      time.Duration

	mu        sync.RWMutex
	callGroup singleflight.Group[K, V]
}

func (c *Cache[K, V]) Get(ctx context.Context, key K, fn func() (V, error)) (V, error) {
	return c.get(ctx, key, false, fn)
}

func (c *Cache[K, V]) GetFresh(ctx context.Context, key K, fn func() (V, error)) (V, error) {
	return c.get(ctx, key, true, fn)
}

func (c *Cache[K, V]) Set(ctx context.Context, key K, fn func() (V, error)) (V, bool, error) {
	v, err, shared := c.callGroup.Do(key, c.set(key, fn))
	return v, shared, err
}

func (c *Cache[K, V]) get(ctx context.Context, key K, freshOnly bool, fn func() (V, error)) (V, error) {
	c.mu.RLock()
	val, ok := c.values.Get(key)
	c.mu.RUnlock()

	// value exists and is fresh - just return
	if ok && val.IsFresh() {
		return val.Value(), nil
	}

	// value exists and is stale, and we're OK with serving it stale while updating in the background
	// note: stale means its still okay, but not fresh. but if its expired, then it means its useless.
	if ok && !freshOnly && !val.IsExpired() {
		// TODO: technically could be a stampede of goroutines here if the value is expired
		// and we're OK with serving it stale
		go c.Set(ctx, key, fn)
		return val.Value(), nil
	}

	// value doesn't exist or is expired, or is stale and we need it fresh (freshOnly:true) - sync update
	v, _, err := c.Set(ctx, key, fn)
	return v, err
}

func (c *Cache[K, V]) set(key K, fn func() (V, error)) func() (V, error) {
	return func() (V, error) {
		val, err := fn()
		if err != nil {
			return val, err
		}

		c.mu.Lock()
		c.values.Add(key, value[V]{
			v:          val,
			expiry:     time.Now().Add(c.ttl),
			bestBefore: time.Now().Add(c.freshFor),
		})
		c.mu.Unlock()

		return val, nil
	}
}

type value[V any] struct {
	v V

	bestBefore time.Time // cache entry freshness cutoff
	expiry     time.Time // cache entry time to live cutoff
}

func (v *value[V]) IsFresh() bool {
	return v.bestBefore.After(time.Now())
}

func (v *value[V]) IsExpired() bool {
	return v.expiry.Before(time.Now())
}

func (v *value[V]) Value() V {
	return v.v
}

func BytesToHash(b ...[]byte) uint64 {
	d := xxh3.New()
	if len(b) == 0 {
		return 0
	}
	if len(b) == 1 {
		d.Write(b[0])
	} else {
		for _, v := range b {
			d.Write(v)
		}
	}
	return d.Sum64()
}

func StringToHash(s ...string) uint64 {
	d := xxh3.New()
	if len(s) == 0 {
		return 0
	}
	if len(s) == 1 {
		d.WriteString(s[0])
	} else {
		for _, v := range s {
			d.WriteString(v)
		}
	}
	return d.Sum64()
}
