package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
)

const (
	// HeaderCorrelationID header key used for correlationID.
	HeaderCorrelationID = "X-Correlation-ID"
)

var (
	// ErrAdvancingMessage the error returned when an attempt is made to advance a event.Message beyond its number of event.Step.
	ErrAdvancingMessage = errors.New("event message: error advancing message next position is greater than number of routes")
	// ErrMaxRetriesExceeded the error returned then a message max retries is exceeded.
	ErrMaxRetriesExceeded = errors.New("event message: error max retries exceeded")
)

type (

	// Message is a struct contain the message information and data.
	Message struct {
		ID        uuid.UUID   `json:"id"`
		Type      MessageType `json:"type"`
		Timestamp time.Time   `json:"timestamp"`
		Headers   Headers     `json:"headers"`
		Routing   *Routing    `routing:"routing"`
		Body      string      `json:"body"`
	}

	// MessageType a message type.
	MessageType string

	// Headers hold any headers associated with the message.
	Headers map[string]string

	// Routing is an optional field and defines the route a message can take through the system.
	Routing struct {
		Position      int           `json:"position"`
		Slip          []Step        `json:"slip"`
		ErrorHandling ErrorHandling `json:"errorHandling"`
	}

	// Step represents a single stage in a message route.
	Step struct {
		Stream  string `json:"stream"`
		OnError string `json:"onError"`
	}

	// ErrorHandling defines the error handling policy for a message.
	ErrorHandling struct {
		MaxRetries int `json:"maxRetries"`
		Attempt    int `json:"attempt"`
	}
)

// NewMessage returns a new event.Message given an event.MessageType and a body.
// The provided body will be serialized into a string.
// The returned event.Message will have an empty Headers and no Routing.
func NewMessage(messageType MessageType, body interface{}) (*Message, error) {
	message := Message{
		ID:        uuid.NewV4(),
		Type:      messageType,
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

// CurrentStep returns the current step the message has reached from its routes.
func (m *Message) CurrentStep() Step {
	return m.Routing.Slip[m.Routing.Position]
}

// Advance resets the error attempts and advances message to the next step.
func (m *Message) Advance() error {
	if m.Routing.Position+1 >= len(m.Routing.Slip) {
		return ErrAdvancingMessage
	}
	m.Routing.ErrorHandling.Attempt = 0
	m.Routing.Position++
	return nil
}

// IncrementErrorAttempt increments the number of attempts to process message.
// if number of attempts is equal to or greater than max retries returns ErrMaxRetriesExceeded.
func (m *Message) IncrementErrorAttempt() error {
	if m.Routing.ErrorHandling.Attempt+1 > m.Routing.ErrorHandling.MaxRetries {
		return ErrMaxRetriesExceeded
	}
	m.Routing.ErrorHandling.Attempt++
	return nil
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
