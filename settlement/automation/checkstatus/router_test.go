package checkstatus

import (
	"fmt"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestRouter_HasRouter(t *testing.T) {
	expected := event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Type:      event.MessageType(testutils.RandomString()),
		Routing: &event.Routing{
			Position: 0,
			Slip: []event.Step{
				{
					Stream:  testutils.RandomString(),
					OnError: testutils.RandomString(),
				},
			},
			ErrorHandling: event.ErrorHandling{
				MaxRetries: 5,
				Attempt:    0,
			},
		},
		Body: testutils.RandomString(),
	}

	err := checkStatusRouter(&expected)
	assert.NoError(t, err)

	// assert no route added
	assert.Equal(t, expected, expected)
}

func TestRouter_HasNoRouter(t *testing.T) {
	expected := event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Type:      event.Grants,
		Body:      testutils.RandomString(),
	}
	err := checkStatusRouter(&expected)
	assert.EqualError(t, err, fmt.Sprintf("check status router: error no route attached for messageID %s", expected.ID))
}
