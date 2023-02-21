package payments

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WorkerConfig defines the settlement worker configuration structure
type WorkerConfig struct {
	PayoutID string `json:"payoutId"`
	Stream   string `json:"stream"`
	Count    int    `json:"count"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (wc WorkerConfig) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(wc)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// prepareWrapper defines the settlement worker prepare message structure
type prepareWrapper struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Body      string    `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (pw prepareWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(pw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// submitWrapper defines the settlement worker submit message structure
type submitWrapper struct {
	ID            uuid.UUID `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Host          string    `json:"host"`
	Digest        string    `json:"digest"`
	Signature     string    `json:"signature"`
	ContentLength string    `json:"contentLength"`
	ContentType   string    `json:"contentType"`
	Body          string    `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (sw submitWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(sw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}
