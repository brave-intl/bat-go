package payments

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WorkerConfig defines the settlement worker configuration structure
type WorkerConfig struct {
	PayoutID      string `json:"payoutId"`
	ConsumerGroup string `json:"consumerGroup"`
	Stream        string `json:"stream"`
	Count         int    `json:"count"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (wc WorkerConfig) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(wc)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// PrepareWrapper defines the settlement worker prepare message structure
type PrepareWrapper struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Body      string    `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (pw PrepareWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(pw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// SubmitWrapper defines the settlement worker submit message structure
type SubmitWrapper struct {
	ID        uuid.UUID         `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (sw SubmitWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(sw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}
