package internal

import (
	"encoding/json"
	"fmt"
)

// WorkerConfig - configure the prepare workers
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
