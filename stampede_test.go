package stampede_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/stampede"
	cachestore "github.com/goware/cachestore2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleflightDo(t *testing.T) {
	s := stampede.NewStampede[int](nil)

	var numCalls atomic.Int64

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		v, err := s.Do("t1", func() (int, *time.Duration, error) {
			numCalls.Add(1)
			time.Sleep(1 * time.Second)
			return 1, nil, nil
		}, stampede.WithTTL(1*time.Second))
		assert.NoError(t, err)
		assert.Equal(t, 1, v)
	}()

	// slight delay, to ensure first call is in flight
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			v, err := s.Do("t1", func() (int, *time.Duration, error) {
				numCalls.Add(1)
				return i, nil, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, 1, v)
		}()
	}

	wg.Wait()

	require.Equal(t, int64(1), numCalls.Load())
}

func newMockCacheBackend() cachestore.Backend {
	return &mockCacheBackend[any]{
		cache: make(map[string]any),
	}
}

type mockCacheBackend[V any] struct {
	cache map[string]V
}

var _ cachestore.Backend = &mockCacheBackend[any]{}

func (m *mockCacheBackend[V]) Name() string {
	return "mockCacheBackend"
}

func (m *mockCacheBackend[V]) Options() cachestore.StoreOptions {
	return cachestore.StoreOptions{}
}

func (m *mockCacheBackend[V]) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := m.cache[key]
	return ok, nil
}

func (m *mockCacheBackend[V]) Set(ctx context.Context, key string, value V) error {
	m.cache[key] = value
	return nil
}

func (m *mockCacheBackend[V]) SetEx(ctx context.Context, key string, value V, ttl time.Duration) error {
	m.cache[key] = value
	return nil
}

func (m *mockCacheBackend[V]) BatchSet(ctx context.Context, keys []string, values []V) error {
	for i, key := range keys {
		m.cache[key] = values[i]
	}
	return nil
}

func (m *mockCacheBackend[V]) BatchSetEx(ctx context.Context, keys []string, values []V, ttl time.Duration) error {
	for i, key := range keys {
		m.cache[key] = values[i]
	}
	return nil
}

func (m *mockCacheBackend[V]) Get(ctx context.Context, key string) (V, bool, error) {
	v, ok := m.cache[key]
	return v, ok, nil
}

func (m *mockCacheBackend[V]) BatchGet(ctx context.Context, keys []string) ([]V, []bool, error) {
	values := make([]V, len(keys))
	exists := make([]bool, len(keys))
	var err error
	for i, key := range keys {
		values[i], exists[i], err = m.Get(ctx, key)
		if err != nil {
			return nil, nil, err
		}
	}
	return values, exists, nil
}

func (m *mockCacheBackend[V]) Delete(ctx context.Context, key string) error {
	delete(m.cache, key)
	return nil
}

func (m *mockCacheBackend[V]) DeletePrefix(ctx context.Context, keyPrefix string) error {
	for key := range m.cache {
		if strings.HasPrefix(key, keyPrefix) {
			delete(m.cache, key)
		}
	}
	return nil
}

func (m *mockCacheBackend[V]) ClearAll(ctx context.Context) error {
	m.cache = make(map[string]V)
	return nil
}

func (m *mockCacheBackend[V]) GetOrSetWithLock(ctx context.Context, key string, getter func(context.Context, string) (V, error)) (V, error) {
	var v V
	return v, fmt.Errorf("not implemented")
}

func (m *mockCacheBackend[V]) GetOrSetWithLockEx(ctx context.Context, key string, getter func(context.Context, string) (V, error), ttl time.Duration) (V, error) {
	var v V
	return v, fmt.Errorf("not implemented")
}
