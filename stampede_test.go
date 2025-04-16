package stampede_test

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
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
	s := stampede.NewStampede[int](slog.Default(), nil)

	var numCalls atomic.Int64

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		v, err := s.Do(context.Background(), "t1", func() (int, *time.Duration, error) {
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

			v, err := s.Do(context.Background(), "t1", func() (int, *time.Duration, error) {
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

func TestCachedDo(t *testing.T) {
	var count uint64
	stampede := stampede.NewStampede(slog.Default(), newMockCacheBackend(), stampede.WithTTL(5*time.Second))

	// repeat test multiple times
	for x := 0; x < 5; x++ {
		// time.Sleep(1 * time.Second)

		var wg sync.WaitGroup
		n := 10
		ctx := context.Background()

		for i := 0; i < n; i++ {
			t.Logf("numGoroutines now %d", runtime.NumGoroutine())

			wg.Add(1)
			go func() {
				defer wg.Done()

				val, err := stampede.Do(ctx, "t1", func() (any, *time.Duration, error) {
					t.Log("cache.Get(t1, ...)")

					// some extensive op..
					time.Sleep(2 * time.Second)
					atomic.AddUint64(&count, 1)

					return "result1", nil, nil
				})

				assert.NoError(t, err)
				assert.Equal(t, "result1", val)
			}()
		}

		wg.Wait()

		// ensure single call
		assert.Equal(t, uint64(1), count)

		// confirm same before/after num of goroutines
		t.Logf("numGoroutines now %d", runtime.NumGoroutine())
	}
}

func newMockCacheBackend() cachestore.Backend {
	return &mockCacheBackend[any]{
		cache:  make(map[string]any),
		expiry: make(map[string]int64),
	}
}

type mockCacheBackend[V any] struct {
	cache  map[string]V
	expiry map[string]int64
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
	m.expiry[key] = time.Now().Unix() + int64(ttl.Seconds())
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
		m.expiry[key] = time.Now().Unix() + int64(ttl.Seconds())
	}
	return nil
}

func (m *mockCacheBackend[V]) Get(ctx context.Context, key string) (V, bool, error) {
	v, ok := m.cache[key]
	if ok {
		expiry, ok := m.expiry[key]
		if ok && expiry < time.Now().Unix() {
			delete(m.cache, key)
			delete(m.expiry, key)
			var v V
			return v, false, nil
		}
	}
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
		if exists[i] {
			expiry, ok := m.expiry[key]
			if ok && expiry < time.Now().Unix() {
				exists[i] = false
				var v V
				values[i] = v
			}
		}
	}
	return values, exists, nil
}

func (m *mockCacheBackend[V]) Delete(ctx context.Context, key string) error {
	delete(m.cache, key)
	delete(m.expiry, key)
	return nil
}

func (m *mockCacheBackend[V]) DeletePrefix(ctx context.Context, keyPrefix string) error {
	for key := range m.cache {
		if strings.HasPrefix(key, keyPrefix) {
			delete(m.cache, key)
			delete(m.expiry, key)
		}
	}
	return nil
}

func (m *mockCacheBackend[V]) ClearAll(ctx context.Context) error {
	m.cache = make(map[string]V)
	m.expiry = make(map[string]int64)
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
