package consumer

import (
	"encoding/json"
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
)

type (
	// Message holds message information and its data.
	Message struct {
		ID        uuid.UUID `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Headers   Headers   `json:"headers"`
		Body      string    `json:"body"`
	}

	// Headers holds headers associated with the message.
	Headers map[string]string
)

// NewMessage returns a new Message with the given body.
// The provided body will be serialized and stored as a string.
// The returned Message will have an empty Headers field.
func NewMessage(body interface{}) (Message, error) {
	message := Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now().UTC(),
		Headers:   make(Headers),
	}

	s, err := json.Marshal(body)
	if err != nil {
		return message, fmt.Errorf("error creating new message: %w", err)
	}

	message.Body = string(s)

	return message, nil
}

// NewMessageFromString deserializes the data and returns a new Message.
// Data must be a valid json Message.
func NewMessageFromString(data string) (Message, error) {
	var message Message
	err := json.Unmarshal([]byte(data), &message)
	if err != nil {
		return message, fmt.Errorf("error creating new message: %w", err)
	}

	if message.Headers == nil {
		message.Headers = make(Headers)
	}

	return message, nil
}

// SetHeader stores the key-value pair in the message Headers.
func (m *Message) SetHeader(key string, value string) {
	m.Headers[key] = value
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis.
func (m Message) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("error marshalling binary: %w", err)
	}
	return bytes, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler required for go-redis.
func (m *Message) UnmarshalBinary(data []byte) error {
	err := json.Unmarshal(data, m)
	if err != nil {
		return fmt.Errorf("error unmarshalling binary: %w", err)
	}
	return nil
}
