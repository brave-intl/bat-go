package status

import (
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// statusRouter a route should have already been attached to the message by prepare stream.
// messages should not be sent directly to the event.CheckStatusStream.
func statusRouter(message *event.Message) error {
	if message.Routing == nil {
		return fmt.Errorf("status router: error no route attached for messageID %s", message.ID)
	}
	return nil
}
