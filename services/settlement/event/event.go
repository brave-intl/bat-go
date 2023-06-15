package event

import (
	"encoding/json"
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
)

type (
	// Message is a struct contain the message information and data.
	Message struct {
		ID        uuid.UUID `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Headers   Headers   `json:"headers"`
		Body      string    `json:"body"`
	}

	// Headers hold any headers associated with the message.
	Headers map[string]string
)

// NewMessage returns a new event.Message with the given body.
// The provided body will be serialized into a string.
// The returned event.Message will have an empty Headers field.
func NewMessage(body interface{}) (*Message, error) {
	message := Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Headers:   make(Headers),
	}
	err := message.SetBody(body)
	if err != nil {
		return nil, fmt.Errorf("event message: error creating new message: %w", err)
	}
	return &message, nil
}

// NewMessageFromString returns a new event.Message deserialized from the given data.
// Data must be valid event.Message json.
func NewMessageFromString(data string) (*Message, error) {
	message := new(Message)
	err := json.Unmarshal([]byte(data), message)
	if err != nil {
		return nil, fmt.Errorf("event message: error creating new message: %w", err)
	}

	if message.Headers == nil {
		message.Headers = make(Headers)
	}

	return message, nil
}

// SetBody sets the body of the message.
func (m *Message) SetBody(body interface{}) error {
	s, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("event message: error marshalling body: %w", err)
	}
	m.Body = string(s)
	return nil
}

// SetHeader stores the key-value pair in the message Headers.
func (m *Message) SetHeader(key string, value string) {
	m.Headers[key] = value
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis.
func (m Message) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler required for go-redis.
func (m *Message) UnmarshalBinary(data []byte) error {
	err := json.Unmarshal(data, m)
	if err != nil {
		return fmt.Errorf("event message: error unmarshalling binary: %w", err)
	}
	return nil
}
