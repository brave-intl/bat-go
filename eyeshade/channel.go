package eyeshade

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	providerRE = regexp.MustCompile(`/^([A-Za-z0-9][A-Za-z0-9-]{0,62})#([A-Za-z0-9][A-Za-z0-9-]{0,62}):(([A-Za-z0-9-._~]|%[0-9A-F]{2})+)$/`)
	youtubeRE  = regexp.MustCompile(`/^UC[0-9A-Za-z_-]{21}[AQgw]$/i`)
)

// ChannelProps holds properties about the channel
type ChannelProps struct {
	Publisher      Channel
	PublisherType  string
	ProviderName   string
	ProviderSuffix string
	ProviderValue  string
	URL            string
	TLD            string
	SLD            string
	RLD            string
	QLD            string
}

// Channel is a type for the channel or sometimes called publisher value
type Channel string

// String converts a channel to a string type
func (ch Channel) String() string {
	return string(ch)
}

// ParseProvider parses provider values out of the channel
func (ch Channel) ParseProvider() (string, string, string) {
	hashSplit := strings.Split(ch.String(), "#")
	name, extra := hashSplit[0], hashSplit[1]
	colonSplit := strings.Split(extra, ":")
	return name, colonSplit[0], colonSplit[1]
}

// Normalize normalizes the channel, parsing out its properties
func (ch Channel) Normalize() Channel {
	props := ch.Props()
	if props.ProviderName == "twitch" {
		return Channel(fmt.Sprintf("%s#author:%s", props.ProviderName, props.ProviderValue))
	} else if props.ProviderName == "youtube" && !ch.ValidYoutubeChannelID() {
		return Channel(fmt.Sprintf("%s#user:%s", props.ProviderName, props.ProviderValue))
	}
	return ch
}

// ValidYoutubeChannelID checks if a youtube id is valid
func (ch Channel) ValidYoutubeChannelID() bool {
	return youtubeRE.Match([]byte(ch.Props().ProviderValue))
}

// Props gets the publisher props
func (ch Channel) Props() *ChannelProps {
	found := providerRE.FindAllString(ch.String(), -1)
	if len(found) == 0 {
		return &ChannelProps{
			URL: ch.String(),
		}
	}
	name, suffix, value := ch.ParseProvider()
	return &ChannelProps{
		Publisher:      ch,
		PublisherType:  "provider",
		ProviderName:   name,
		ProviderSuffix: suffix,
		ProviderValue:  value,
		TLD:            name + "#" + suffix,
		SLD:            ch.String(),
		RLD:            value,
	}
}
