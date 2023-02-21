package payments

import (
	"context"

	"github.com/schollz/progressbar/v3"
)

type result struct {
	Err error
}

func pipeline[T any](ctx context.Context, numWorkers, numRecords int, action func(T) error, records ...T) error {
	bar := progressbar.Default(int64(numRecords))
	// fan out coordinator
	in := make(chan T, numRecords/numWorkers)
	go coordinator(in, records...)

	out := make(chan result)
	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
	}()

	// spin up workers
	for i := 0; i < numWorkers; i++ {
		go worker(ctx, action, in, out)
	}

	// wait for workers to complete, if we get an error kill remaining workers (deferred)
	for j := 0; j < numRecords; j++ {
		if result := <-out; result.Err != nil {
			// return the error
			return result.Err
		}
		bar.Add(1)
	}
	return nil
}

// coordinator is a generic fan out coordinator that dumps items on out chan
func coordinator[T any](out chan T, items ...T) {
	for _, v := range items {
		out <- v
	}
}

// worker is a pipeline worker that runs a Tx|AttestedTx processing function
func worker[T any](ctx context.Context, process func(T) error, in <-chan T, out chan result) {
	for {
		select {
		case m := <-in:
			if err := process(m); err != nil {
				out <- result{Err: err}
				return
			}
			out <- result{}
		case <-ctx.Done():
			return
		}
	}
}
