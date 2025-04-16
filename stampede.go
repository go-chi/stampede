package stampede

import (
	"context"
	"fmt"
	"sync"
	"time"

	cachestore "github.com/goware/cachestore2"
	"github.com/goware/singleflight"
	"github.com/zeebo/xxh3"
)

const (
	// DefaultCacheTTL is the default TTL for cache entries. However,
	// you can pass WithTTL(d) to set your own ttl, or pass
	// WithSkipCache() to disable caching
	DefaultCacheTTL = 1 * time.Minute
)

func NewStampede[V any](cache cachestore.Store[V], options ...Option) *stampede[V] {
	opts := &Options{}
	for _, o := range options {
		o(opts)
	}

	return &stampede[V]{
		cache:     cache,
		callGroup: singleflight.Group[string, V]{},
		options:   opts,
	}
}

type stampede[V any] struct {
	cache     cachestore.Store[V]
	callGroup singleflight.Group[string, V]
	options   *Options
	mu        sync.RWMutex
}

func (s *stampede[V]) Do(key string, fn func() (V, error), options ...Option) (V, error) {
	opts := &Options{}
	for _, o := range options {
		o(opts)
	}
	if len(options) == 0 {
		opts = s.options
	}

	// TODO: what happens if we have a panic in the fn ..?

	if opts.SkipCache || s.cache == nil {
		// Singleflight mode only
		v, err, _ := s.callGroup.Do(key, func() (V, error) {
			return fn()
		})
		return v, err
	} else {
		// Caching + Singleflight combo mode

		// TODO: handle if we have a panic..?

		s.mu.RLock()
		v, ok, err := s.cache.Get(context.Background(), key)
		if err != nil {
			s.mu.RUnlock()
			return v, err
		}
		s.mu.RUnlock()
		if ok {
			fmt.Println("cache hit", v)
			return v, nil
		}

		v, err, _ = s.callGroup.Do(key, func() (V, error) {
			return fn()
		})
		if err != nil {
			return v, err
		}

		ttl := opts.TTL
		if ttl == 0 {
			ttl = DefaultCacheTTL
		}

		s.mu.Lock()
		err = s.cache.SetEx(context.Background(), key, v, ttl)
		if err != nil {
			s.mu.Unlock()
			return v, err // TODO: maybe just log this error instead ..?
		}
		s.mu.Unlock()
		return v, nil
	}
}

func (s *stampede[V]) SetOptions(options *Options) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.options = options
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
