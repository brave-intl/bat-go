package submit

import (
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// submitRouter a route should have already been attached to the message by prepare stream.
// messages should not be sent directly to the event.SubmitStream.
func submitRouter(message *event.Message) error {
	if message.Routing == nil {
		return fmt.Errorf("submit router: error no route attached for messageID %s", message.ID)
	}
	return nil
}
