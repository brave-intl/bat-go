package prepare

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
		Type:      event.Grants,
		Routing: &event.Routing{
			Position: 0,
			Slip: []event.Step{
				{
					Stream:  event.SubmitStatusStream,
					OnError: event.ErroredStream,
				},
			},
			ErrorHandling: event.ErrorHandling{
				MaxRetries: 5,
				Attempt:    0,
			},
		},
		Body: testutils.RandomString(),
	}

	err := prepareRouter(&expected)
	assert.NoError(t, err)

	// assert no route added
	assert.Equal(t, expected, expected)
}

func TestRouter_Grants(t *testing.T) {
	actual := event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Type:      event.Grants,
		Body:      testutils.RandomString(),
	}

	err := prepareRouter(&actual)
	assert.NoError(t, err)

	expected := &event.Routing{
		Position: 0,
		Slip: []event.Step{
			{
				Stream:  event.PrepareStream,
				OnError: event.ErroredStream,
			},
			{
				Stream:  event.SubmitStream,
				OnError: event.ErroredStream,
			},
			{
				Stream:  event.SubmitStatusStream,
				OnError: event.ErroredStream,
			},
			{
				Stream:  event.CheckStatusStream,
				OnError: event.ErroredStream,
			},
		},
		ErrorHandling: event.ErrorHandling{
			MaxRetries: 5,
			Attempt:    0,
		},
	}

	assert.Equal(t, expected, actual.Routing)
}

func TestRouter_Ads(t *testing.T) {
	actual := event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Type:      event.Ads,
		Body:      testutils.RandomString(),
	}

	err := prepareRouter(&actual)
	assert.NoError(t, err)

	expected := &event.Routing{
		Position: 0,
		Slip: []event.Step{
			{
				Stream:  event.PrepareStream,
				OnError: event.ErroredStream,
			},
			{
				Stream:  event.NotifyStream,
				OnError: event.ErroredStream,
			},
		},
		ErrorHandling: event.ErrorHandling{
			MaxRetries: 5,
			Attempt:    0,
		},
	}
	assert.Equal(t, expected, actual.Routing)
}

func TestRouter_UnknownType(t *testing.T) {
	actual := event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Type:      event.MessageType(testutils.RandomString()),
		Body:      testutils.RandomString(),
	}
	err := prepareRouter(&actual)
	assert.EqualError(t, err, fmt.Sprintf("prepare router: error unknown message type %s", actual.Type))
}
