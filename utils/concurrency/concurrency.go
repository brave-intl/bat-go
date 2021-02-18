package concurrency

import (
	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

// Concurrently holds the info about the parallel calls
type Concurrently struct {
	errors     chan error
	cachedErrs *errorutils.MultiError
	limiter    chan int
}

// New creates a new concurrently struct
func New(limit int) *Concurrently {
	return &Concurrently{
		make(chan error),
		new(errorutils.MultiError),
		make(chan int, limit),
	}
}

// Next waits for the next slot to be open
func (cc *Concurrently) Next(target int) {
	cc.limiter <- target
}

// Finish waits for the finish slot to be open
func (cc *Concurrently) Finish() {
	<-cc.limiter
}

// AddError collects errors
func (cc *Concurrently) Error(err error) {
	cc.errors <- err
}

// Errors gets errors as an a multierror
func (cc *Concurrently) Errors() error {
	me := cc.cachedErrs
	errorLength := len(cc.errors)
	if errorLength > 0 {
		total := errorLength + me.Count()
		for {
			err := <-cc.errors
			me.Append(err)
			if me.Count() == total {
				break
			}
		}
		cc.cachedErrs = me
	}
	if me.Count() == 0 {
		return nil
	}
	return me
}
