package event

import (
	"encoding/json"
	"testing"
	"time"

	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestMessage_NewMessageFromString(t *testing.T) {
	expected := createMessage()

	bytes, err := json.Marshal(expected)
	assert.NoError(t, err)

	data := string(bytes)

	actual, err := NewMessageFromString(data)
	assert.NoError(t, err)

	assert.Equal(t, expected, actual)
}

func TestMessage_SetBody(t *testing.T) {
	body := testutils.RandomString()

	message := &Message{
		Body: testutils.RandomString(),
	}
	assert.NotEqual(t, body, message.Body)

	err := message.SetBody(body)
	assert.NoError(t, err)

	expected, err := json.Marshal(body)
	assert.NoError(t, err)

	assert.Equal(t, string(expected), message.Body)
}

func TestMessage_CurrentStep(t *testing.T) {
	expected := testutils.RandomInt()
	message := &Message{
		Routing: &Routing{
			Position: expected,
		},
	}
	assert.Equal(t, expected, message.Routing.Position)
}

func TestMessage_Advance_Error(t *testing.T) {
	position := testutils.RandomInt()
	message := &Message{
		Routing: &Routing{
			Position: position,
		},
	}
	assert.Equal(t, position, message.Routing.Position)

	err := message.Advance()

	assert.EqualError(t, err, ErrAdvancingMessage.Error())
	assert.Equal(t, position, message.Routing.Position)
}

func TestMessage_Advance(t *testing.T) {
	position := 0
	message := &Message{
		Routing: &Routing{
			Slip: []Step{
				{
					Stream:  testutils.RandomString(),
					OnError: testutils.RandomString(),
				},
				{
					Stream:  testutils.RandomString(),
					OnError: testutils.RandomString(),
				},
			},
			Position: position,
		},
	}
	assert.Equal(t, position, message.Routing.Position)

	err := message.Advance()

	assert.NoError(t, err)
	assert.Equal(t, position+1, message.Routing.Position)
}

func TestMessage_IncrementErrorAttempt_Error(t *testing.T) {
	maxRetries := 0
	message := &Message{
		Routing: &Routing{
			ErrorHandling: ErrorHandling{
				MaxRetries: maxRetries,
			},
		},
	}
	assert.Equal(t, maxRetries, message.Routing.ErrorHandling.Attempt)

	err := message.IncrementErrorAttempt()
	assert.EqualError(t, err, ErrMaxRetriesExceeded.Error())
}

func TestMessage_IncrementErrorAttempt(t *testing.T) {
	maxRetries := 1
	message := &Message{
		Routing: &Routing{
			ErrorHandling: ErrorHandling{
				Attempt:    0,
				MaxRetries: maxRetries,
			},
		},
	}
	assert.NotEqual(t, maxRetries, message.Routing.ErrorHandling.Attempt)

	err := message.IncrementErrorAttempt()
	assert.NoError(t, err)
	assert.Equal(t, 1, message.Routing.ErrorHandling.Attempt)
}

func TestMessage_MarshalBinary(t *testing.T) {
	message := createMessage()

	expected, err := json.Marshal(message)
	assert.NoError(t, err)

	actual, err := message.MarshalBinary()
	assert.NoError(t, err)

	assert.Equal(t, expected, actual)
}

func TestMessage_UnmarshalBinary(t *testing.T) {
	expected := createMessage()

	data, err := json.Marshal(expected)
	assert.NoError(t, err)

	actual := Message{}
	err = actual.UnmarshalBinary(data)

	assert.NoError(t, err)
	assert.Equal(t, expected, &actual)
}

func createMessage() *Message {
	timestamp := time.Time{}
	headers := Headers{}
	headers[testutils.RandomString()] = testutils.RandomString()
	routing := &Routing{
		Position: testutils.RandomInt(),
		Slip: []Step{
			{
				Stream:  testutils.RandomString(),
				OnError: testutils.RandomString(),
			},
			{
				Stream:  testutils.RandomString(),
				OnError: testutils.RandomString(),
			},
		},
		ErrorHandling: ErrorHandling{
			MaxRetries: testutils.RandomInt(),
			Attempt:    testutils.RandomInt(),
		},
	}
	body := testutils.RandomString()

	return &Message{
		ID:        uuid.NewV4(),
		Type:      MessageType(testutils.RandomString()),
		Timestamp: timestamp,
		Headers:   headers,
		Routing:   routing,
		Body:      body,
	}
}
