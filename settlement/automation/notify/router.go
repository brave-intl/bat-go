package notify

import (
	"fmt"
	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// notifyRouter a route should have already been attached to the message by prepare stream.
func notifyRouter(message *event.Message) error {
	if message.Routing == nil {
		return fmt.Errorf("notify router: error no route attached for messageID %s", message.ID)
	}
	return nil
}
