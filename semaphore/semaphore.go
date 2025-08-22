package semaphore

import (
	"context"
	"sync/atomic"

	msemaphore "github.com/marusama/semaphore/v2"
	"golang.org/x/sync/errgroup"
)

// Semaphore is a wrapper around github.com/marusama/semaphore that counts the number of goroutines that are currently blocked inside Acquire.
type Semaphore struct {
	msemaphore.Semaphore
	waiters atomic.Uint32
}

func New(limit int) Semaphore {
	return Semaphore{Semaphore: msemaphore.New(limit)}
}

// Acquire tries to acquire n resources from the semaphore, blocking until they are available or the context is cancelled.
// If the semaphore is full, the number of waiters is incremented and any provided waiting callbacks are called in separate goroutines.
// If any waiting returns an error, all other waiting callbacks are cancelled and the first error is returned.
func (s *Semaphore) Acquire(ctx context.Context, n int, waitingCallbacks ...func(ctx context.Context) error) error {
	// Fast path: if TryAcquire succeeds, nobody waited.
	// This is attempt to avoid incrementing the wait count momentary if the semaphore is not full.
	if s.TryAcquire(n) {
		return nil
	}

	// going to block
	s.waiters.Add(1)
	defer s.waiters.Add(^uint32(0)) // decrement unsigned integer by 1

	// call waiting callbacks
	g, ctx := errgroup.WithContext(ctx)
	for _, cb := range waitingCallbacks {
		cb := cb // capture range variable
		g.Go(func() error {
			return cb(ctx)
		})
	}

	err := s.Semaphore.Acquire(ctx, n)

	// ensure that all "waiting" callbacks have finished before returning
	if err := g.Wait(); err != nil {
		return err
	}

	return err
}

func (s *Semaphore) Waiters() uint32 {
	return s.waiters.Load()
}
