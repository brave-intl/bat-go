package event

import (
	"encoding/json"
	"errors"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"time"
)

const (
	// HeaderCorrelationID header key used for correlationID
	HeaderCorrelationID = "X-Correlation-ID"
)

var (
	ErrAdvancingMessage   = errors.New("event message: error advancing message next position is greater than number of routes")
	ErrMaxRetriesExceeded = errors.New("event message: error max retries exceeded")
)

type (
	Message struct {
		ID        uuid.UUID   `json:"id"`
		Type      MessageType `json:"type"`
		Timestamp time.Time   `json:"timestamp"`
		Headers   Headers     `json:"headers"`
		Routing   *Routing    `routing:"routing"`
		Body      string      `json:"body"`
	}

	MessageType string

	Headers map[string]string

	Routing struct {
		Position      int           `json:"position"`
		Slip          []Step        `json:"slip"`
		ErrorHandling ErrorHandling `json:"errorHandling"`
	}

	Step struct {
		Stream  string `json:"stream"`
		OnError string `json:"onError"`
	}

	ErrorHandling struct {
		MaxRetries int `json:"maxRetries"`
		Attempt    int `json:"attempt"`
	}
)

// NewMessageFromString creates a new instance of message from string dataKey
func NewMessageFromString(data string) (*Message, error) {
	var message Message
	err := json.Unmarshal([]byte(data), &message)
	if err != nil {
		return nil, fmt.Errorf("event message: error creating new message: %w", err)
	}

	if message.Headers == nil {
		message.Headers = Headers{}
	}

	return &message, nil
}

// SetBody sets the body of the message
func (m *Message) SetBody(body interface{}) error {
	s, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("event message: error marshalling body: %w", err)
	}
	m.Body = string(s)
	return nil
}

// CurrentStep returns the current step the message has reached from its routes
func (m *Message) CurrentStep() Step {
	return m.Routing.Slip[m.Routing.Position]
}

// Advance resets the error attempts and advances message to the next step
func (m *Message) Advance() error {
	if m.Routing.Position+1 >= len(m.Routing.Slip) {
		return ErrAdvancingMessage
	}
	m.Routing.ErrorHandling.Attempt = 0
	m.Routing.Position += 1
	return nil
}

// IncrementErrorAttempt increments the number of attempts to process message
// if number of attempts is equal to or greater than max retries returns ErrMaxRetriesExceeded
func (m *Message) IncrementErrorAttempt() error {
	if m.Routing.ErrorHandling.Attempt+1 > m.Routing.ErrorHandling.MaxRetries {
		return ErrMaxRetriesExceeded
	}
	m.Routing.ErrorHandling.Attempt += 1
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (m Message) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler required for go-redis
func (m *Message) UnmarshalBinary(data []byte) error {
	err := json.Unmarshal(data, m)
	if err != nil {
		return fmt.Errorf("event message: error unmarshalling binary: %w", err)
	}
	return nil
}
