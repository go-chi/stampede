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

type stampede[V any] struct {
	cache     cachestore.Store[V]
	callGroup singleflight.Group[uint64, V]
	options   *Options
	mu        sync.RWMutex
}

func NewStampede[V any](cache cachestore.Store[V], options ...Option) *stampede[V] {
	opts := &Options{}
	for _, o := range options {
		o(opts)
	}

	return &stampede[V]{
		cache:     cache,
		callGroup: singleflight.Group[uint64, V]{},
		options:   opts,
	}
}

// TODO: maybe always use key as uint64 ..? and we use
// xxhash3 thing..? kinda makes sense to me..

func (s *stampede[V]) Do(key string, fn func() (V, error), options ...Option) (V, error) {
	k := StringToHash(key)
	_ = k

	opts := &Options{}
	for _, o := range options {
		o(opts)
	}
	if len(options) == 0 {
		opts = s.options
	}

	// TODO: what happens if we have a panic in the fn ..?

	if opts.SkipCache || s.cache == nil {
		// TODO: is there a memory leak or something with .Do() ..?
		v, err, _ := s.callGroup.Do(k, func() (V, error) {
			return fn()
		})
		return v, err
	} else {

		// TODO: handle if we have a panic..?

		s.mu.Lock()
		v, ok, err := s.cache.Get(context.Background(), key)
		if err != nil {
			s.mu.Unlock()
			return v, err
		}
		s.mu.Unlock()
		if ok {
			fmt.Println("cache hit", v)
			return v, nil
		}

		// TODO: we can check if v is still fresh.. etc.?

		v, err, _ = s.callGroup.Do(k, func() (V, error) {
			return fn()
		})
		if err != nil {
			return v, err
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		// TODO: maybe we should have CacheTTL pkg-level default..? like 1 minute ..?

		ttl := opts.TTL
		if ttl == 0 {
			ttl = 1 * time.Minute // pkg-level default of 1 min..
		}

		err = s.cache.SetEx(context.Background(), key, v, ttl)
		if err != nil {
			// maybe just log this error instead ..?
			return v, err
		}
		return v, nil

	}
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
