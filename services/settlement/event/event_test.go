package event

import (
	"encoding/json"
	"testing"
	"time"

	testutils "github.com/brave-intl/bat-go/libs/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewMessage(t *testing.T) {
	data := testutils.RandomString()

	actual, err := NewMessage(data)
	assert.NoError(t, err)

	assert.NotEmpty(t, actual.ID)
	assert.Equal(t, Headers{}, actual.Headers)
	assert.WithinDuration(t, time.Now(), actual.Timestamp, 1*time.Second)

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
	body := testutils.RandomString()

	return &Message{
		ID:        uuid.NewV4(),
		Timestamp: timestamp,
		Headers:   headers,
		Body:      body,
	}
}
