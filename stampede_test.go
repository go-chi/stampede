package stampede_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/stampede"
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

		v, err := s.Do("t1", func() (int, error) {
			numCalls.Add(1)
			time.Sleep(1 * time.Second)
			return 1, nil
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

			v, err := s.Do("t1", func() (int, error) {
				numCalls.Add(1)
				return i, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, 1, v)
		}()
	}

	wg.Wait()

	require.Equal(t, int64(1), numCalls.Load())
}
