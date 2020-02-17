package slack

import (
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
)

// Time handles slack's custom time format
type Time time.Time

// UnmarshalJSON parses slack's custom time format
func (t *Time) UnmarshalJSON(data []byte) error {
	// Ignore null, like in the main JSON package.
	if string(data) == "null" {
		return nil
	}
	// Fractional seconds are handled implicitly by Parse.
	var err error
	str := string(data)
	b := len(str) - 1
	str = str[1:b]
	split := strings.Split(str, ".")
	seconds, err := strconv.ParseInt(split[0], 10, 64)
	if err != nil {
		return err
	}
	subseconds, err := strconv.ParseInt(split[1], 10, 64)
	if err != nil {
		return err
	}
	*t = Time(time.Unix(seconds, subseconds))
	return err
}

// String turns the date back into a string
func (t Time) String() string {
	gotime := time.Time(t)
	seconds := strconv.Itoa(int(gotime.Unix()))
	nanoseconds := strconv.Itoa(int(gotime.UnixNano()))
	return seconds + "." + nanoseconds
}

// Text a structure for holding data about a slack placeholder
type Text struct {
	Emoji bool   `json:"emoji"`
	Text  string `json:"text"`
	Type  string `json:"type"`
}

// SideText a structure for holding data about a slack block
type SideText struct {
	Text     string `json:"text"`
	Type     string `json:"type"`
	Verbatim bool   `json:"verbatim"`
}

// Option an interactive slack option
type Option struct {
	Text  Text   `json:"text"`
	Value string `json:"value"`
}

// Team holds information about the team the command came from
type Team struct {
	Domain string `json:"domain"`
	ID     string `json:"id"`
}

// Container holds info about the container
type Container struct {
	Type             string `json:"type"`
	ViewID           string `json:"view_id"`
	MessageTimestamp Time   `json:"message_ts"`
	AttachmentID     int    `json:"attachment_id"`
	ChannelID        string `json:"channel_id"`
	IsEphemeral      bool   `json:"is_ephemeral"`
	IsAppUnfurl      bool   `json:"is_app_unfurl"`
}

// Channel holds identifying information about the channel command was issued in
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// User holds information about the user
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TeamID   string `json:"team_id"`
	Username string `json:"username"`
}

// Action holds an interactive action
type Action struct {
	ActionID        string  `json:"action_id"`
	ActionTimestamp Time    `json:"action_ts"`
	BlockID         string  `json:"block_id"`
	Placeholder     Text    `json:"placeholder"`
	Text            Text    `json:"text"`
	SelectedOption  *Option `json:"selected_option,omitempty"`
	Type            string  `json:"type"`
	SelectedDate    string  `json:"selected_date"`
}

// Element holds info about the actual dom element
type Element struct {
	ActionID    string `json:"action_id"`
	Placeholder Text   `json:"placeholder"`
	Type        string `json:"type"`
	// for checkboxes
	Options *[]Option `json:"options,omitempty"`
}

// Accessory holds information about a sub element of a block
type Accessory struct {
	ActionID    string    `json:"action_id"`
	Placeholder Text      `json:"placeholder"`
	Type        string    `json:"type"`
	Options     *[]Option `json:"options,omitempty"`
}

// Block holds info about an interactive block
type Block struct {
	BlockID   string     `json:"block_id"`
	Element   *Element   `json:"element,omitempty"`
	Hint      *Text      `json:"hint,omitempty"`
	Label     *Text      `json:"label,omitempty"`
	Optional  bool       `json:"optional"`
	Type      string     `json:"type"`
	Text      SideText   `json:"text"`
	Accessory *Accessory `json:"accessory,omitempty"`
}

// InputValue final value of the form actions
type InputValue struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
	// Selected []Option `json:"selected_options,omitempty"`
}

// // BlockValue holds ids to action values
// type BlockValue map[string]InputValue

// // State holds final state of form
// type State map[string]BlockValue

// Message holds message data
type Message struct {
	BotID     string `json:"bot_id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	User      string `json:"user"`
	Timestamp Time   `json:"ts"`
}

// InputValueMap holds a mapping of input values
type InputValueMap map[string]InputValue

// State holds the state of the form
type State struct {
	Values map[string]InputValueMap `json:"values"`
}

// View holds a view of interactive blocks
type View struct {
	AppID              string    `json:"app_id"`
	AppInstalledTeamID string    `json:"app_installed_team_id"`
	Blocks             []Block   `json:"blocks"`
	BotID              string    `json:"bot_id"`
	CallbackID         uuid.UUID `json:"callback_id"`
	ClearOnClose       bool      `json:"clear_on_close"`
	Close              Text      `json:"close"`
	ExternalID         string    `json:"external_id"`
	Hash               string    `json:"hash"`
	ID                 string    `json:"id"`
	NotifyOnClose      bool      `json:"notify_on_close"`
	PrivateMetadata    string    `json:"private_metadata"`
	RootViewID         string    `json:"root_view_id"`
	State              State     `json:"state"`
	Submit             Text      `json:"submit"`
	TeamID             string    `json:"team_id"`
	Title              Text      `json:"title"`
	Type               string    `json:"type"`
}

// ActionsRequest holds request from block actions
type ActionsRequest struct {
	Type         string    `json:"type"`
	Team         Team      `json:"team"`
	User         User      `json:"user"`
	APIAppID     string    `json:"api_app_id"`
	Token        string    `json:"token"`
	Container    Container `json:"container"`
	TriggerID    string    `json:"trigger_id"`
	View         View      `json:"view"`
	Channel      *Channel  `json:"channel,omitempty"`
	Message      *Message  `json:"message,omitempty"`
	ResponseURLs []string  `json:"response_urls"`
	Actions      []Action  `json:"actions"`
}

// BlockActionsResponse is a generic container for block actions
type BlockActionsResponse struct {
	OK   bool `json:"ok"`
	View View `json:"view"`
}

// ViewUpdateRequest holds info for updating modal
type ViewUpdateRequest struct {
	View       string `json:"view"`
	ExternalID string `json:"external_id"`
	Hash       string `json:"hash"`
}
