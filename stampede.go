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
		callGroup: singleflight.Group[string, doResult[V]]{},
		options:   opts,
	}
}

type stampede[V any] struct {
	cache     cachestore.Store[V]
	callGroup singleflight.Group[string, doResult[V]]
	options   *Options
	mu        sync.RWMutex
}

type doResult[V any] struct {
	Value V
	TTL   *time.Duration
}

func (s *stampede[V]) Do(key string, fn func() (V, *time.Duration, error), options ...Option) (V, error) {
	var opts *Options
	if len(options) > 0 {
		opts = getOptions(0, options...)
	} else {
		opts = s.options
	}

	// TODO: what happens if we have a panic in the fn ..?

	key = fmt.Sprintf("stampede:%s", key)

	if opts.SkipCache || s.cache == nil {
		// Singleflight mode only
		result, err, _ := s.callGroup.Do(key, func() (doResult[V], error) {
			v, ttl, err := fn()
			if err != nil {
				return doResult[V]{Value: v, TTL: ttl}, err
			}
			return doResult[V]{Value: v, TTL: ttl}, nil
		})
		return result.Value, err
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

		result, err, _ := s.callGroup.Do(key, func() (doResult[V], error) {
			v, ttl, err := fn()
			if err != nil {
				return doResult[V]{Value: v, TTL: ttl}, err
			}
			return doResult[V]{Value: v, TTL: ttl}, nil
		})

		if err != nil {
			return result.Value, err
		}

		var ttl time.Duration
		if result.TTL != nil {
			ttl = *result.TTL
		} else {
			ttl = opts.TTL
		}

		s.mu.Lock()
		err = s.cache.SetEx(context.Background(), key, result.Value, ttl)
		if err != nil {
			s.mu.Unlock()
			return result.Value, err // TODO: maybe just log this error instead ..?
		}
		s.mu.Unlock()
		return result.Value, nil
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
