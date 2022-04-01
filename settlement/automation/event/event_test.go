package event

import (
	"encoding/json"
	"testing"
	"time"

	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewMessage(t *testing.T) {
	msgType := MessageType(testutils.RandomString())
	data := testutils.RandomString()

	actual, err := NewMessage(msgType, data)
	assert.NoError(t, err)

	assert.NotEmpty(t, actual.ID)
	assert.Equal(t, Headers{}, actual.Headers)
	assert.WithinDuration(t, time.Now(), actual.Timestamp, 1*time.Second)
	assert.Nil(t, actual.Routing)

	var body string
	err = json.Unmarshal([]byte(actual.Body), &body)
	assert.NoError(t, err)
	assert.Equal(t, data, body)
}

func TestNewMessageFromString(t *testing.T) {
	expected := createMessage()

	bytes, err := json.Marshal(expected)
	assert.NoError(t, err)

	data := string(bytes)

	actual, err := NewMessageFromString(data)
	assert.NoError(t, err)

	assert.Equal(t, expected, actual)
}

func TestSetBody(t *testing.T) {
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

func TestCurrentStep(t *testing.T) {
	expected := testutils.RandomInt()
	message := &Message{
		Routing: &Routing{
			Position: expected,
		},
	}
	assert.Equal(t, expected, message.Routing.Position)
}

func TestAdvance_Error(t *testing.T) {
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

func TestAdvance(t *testing.T) {
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

func TestIncrementErrorAttempt_Error(t *testing.T) {
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

func TestIncrementErrorAttempt(t *testing.T) {
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

func TestMarshalBinary(t *testing.T) {
	message := createMessage()

	expected, err := json.Marshal(message)
	assert.NoError(t, err)

	actual, err := message.MarshalBinary()
	assert.NoError(t, err)

	assert.Equal(t, expected, actual)
}

func TestUnmarshalBinary(t *testing.T) {
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
