package semaphore

import (
	"context"
	"sync/atomic"

	msemaphore "github.com/marusama/semaphore/v2"
)

// Semaphore is a wrapper around github.com/marusama/semaphore that counts the number of goroutines that are currently blocked inside Acquire.
type Semaphore struct {
	msemaphore.Semaphore
	waiters atomic.Uint32
}

func New(limit int) Semaphore {
	return Semaphore{Semaphore: msemaphore.New(limit)}
}

func (s *Semaphore) Acquire(ctx context.Context, n int) error {
	// Fast path: if TryAcquire succeeds, nobody waited.
	// This is attempt to avoid incrementing the wait count momentary if the semaphore is not full.
	if s.TryAcquire(n) {
		return nil
	}

	// going to block
	s.waiters.Add(1)
	err := s.Semaphore.Acquire(ctx, n)
	s.waiters.Add(^uint32(0)) // decrement unsigned integer by 1

	return err
}

func (s *Semaphore) Waiters() uint32 {
	return s.waiters.Load()
}
