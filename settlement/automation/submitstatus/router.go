package submitstatus

import (
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// submitStatusRouter defines the steps for messages added to the submit submitStatus stream.
// Ads messages can be directly added to this stream so we need to attach a route.
// Grant message type should already have a route as they should only be added to the prepare stream.
func submitStatusRouter(message *event.Message) error {
	if message.Routing == nil {
		switch message.Type {
		case event.Ads:
			message.Routing = &event.Routing{
				Position: 0,
				Slip: []event.Step{
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
		default:
			return fmt.Errorf("submit status router: error unknown message type %s", message.Type)
		}
	}
	return nil
}
