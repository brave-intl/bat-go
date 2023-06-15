package event

import (
	"context"
	"sync"
)

// NewFanIn return a new fan-in function for type T.
func NewFanIn[T any]() func(ctx context.Context, channels ...<-chan T) <-chan T {
	return func(ctx context.Context, channels ...<-chan T) <-chan T {
		var wg sync.WaitGroup
		multiplexedStream := make(chan T)

		multiplex := func(c <-chan T) {
			defer wg.Done()
			for i := range c {
				select {
				case <-ctx.Done():
					return
				case multiplexedStream <- i:
				}
			}
		}

		wg.Add(len(channels))
		for _, c := range channels {
			go multiplex(c)
		}

		go func() {
			wg.Wait()
			close(multiplexedStream)
		}()

		return multiplexedStream
	}
}
