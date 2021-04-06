package inputs

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTimeDecodeNotValid - an error that tells caller the id is not a uuid
	ErrTimeDecodeNotValid = errors.New("failed to decode timestamp: time is not a valid timestamp")
	// ErrTimeDecodeEmpty - an error that tells caller the id is empty and should not be
	ErrTimeDecodeEmpty = errors.New("failed to decode timestamp: timestamp cannot be empty")
)

// Time - a generic Time type that can be used for common time based things
type Time struct {
	time   *time.Time
	raw    string
	layout string
}

// NewTime creates a new time input
func NewTime(layout string, input ...time.Time) *Time {
	t := new(Time)
	t.layout = layout
	if len(input) > 0 {
		t.SetTime(input[0])
	}
	return t
}

// SetTime sets the time
func (t *Time) SetTime(current time.Time) {
	t.time = &current
	t.raw = current.Format(t.layout)
}

// Time - return the time.Time representation of the parsed time
func (t *Time) Time() *time.Time {
	return t.time
}

// String - return the String representation of the time
func (t *Time) String() string {
	return t.raw
}

// Validate - take raw []byte input and populate id with the ID
func (t *Time) Validate(ctx context.Context) error {
	// this should be overloaded to validate ids are real...
	return nil
}

// Decode - take raw []byte input and populate id with the ID
func (t *Time) Decode(ctx context.Context, input []byte) error {
	var err error

	if len(input) == 0 {
		return ErrTimeDecodeEmpty
	}
	t.raw = string(input)

	var parsed time.Time
	if parsed, err = time.Parse(t.layout, t.raw); err != nil {
		return ErrTimeDecodeNotValid
	}
	t.SetTime(parsed)
	return nil
}
