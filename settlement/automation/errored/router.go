package errored

import (
	"fmt"
	"github.com/brave-intl/bat-go/settlement/automation/event"
)

// erroredRouter a route should have already been attached to the message by a previous stream.
func erroredRouter(message *event.Message) error {
	if message.Routing == nil {
		return fmt.Errorf("errored router: error no route attached for messageID %s", message.ID)
	}
	return nil
}
