package checkstatus

import (
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// checkStatusRouter a route should have already been attached to the message by prepare stream.
// messages should not be sent directly to the event.CheckStatusStream.
func checkStatusRouter(message *event.Message) error {
	if message.Routing == nil {
		return fmt.Errorf("check status router: error no route attached for messageID %s", message.ID)
	}
	return nil
}
