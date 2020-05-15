package stampede_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goware/stampede"
	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {

	var count uint64
	cache := stampede.NewCache(time.Duration(2*time.Second), time.Duration(5*time.Second))

	// repeat test multiple times
	for x := 0; x < 5; x++ {
		// time.Sleep(1 * time.Second)

		var wg sync.WaitGroup
		numGoroutines := runtime.NumGoroutine()

		n := 10
		ctx := context.Background()

		for i := 0; i < n; i++ {
			t.Logf("numGoroutines now %d", runtime.NumGoroutine())

			wg.Add(1)
			go func() {
				defer wg.Done()

				val, err := cache.Get(ctx, "t1", func(ctx context.Context) (interface{}, error) {
					t.Log("cache.Get(t1, ...)")

					// some extensive op..
					time.Sleep(2 * time.Second)
					atomic.AddUint64(&count, 1)

					return "result1", nil
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
		assert.Equal(t, numGoroutines, runtime.NumGoroutine())

	}
}
