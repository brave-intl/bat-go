package prepare

import (
	"fmt"
	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// prepareRouter defines the steps for messages added to the prepare stream.
// Grants messages do not require an external verification step so can skip notification.
// Ads messages need to flow to the notify step once they have been prepared.
func prepareRouter(message *event.Message) error {
	if message.Routing == nil {
		switch message.Type {
		case event.Grants:
			message.Routing = &event.Routing{
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
		case event.Ads, event.Creators:
			message.Routing = &event.Routing{
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
		default:
			return fmt.Errorf("prepare router: error unknown message type %s", message.Type)
		}
	}
	return nil
}
