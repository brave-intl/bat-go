package eyeshade

import (
	"regexp"
	"strings"
)

var (
	providerRE = regexp.MustCompile(`/^([A-Za-z0-9][A-Za-z0-9-]{0,62})#([A-Za-z0-9][A-Za-z0-9-]{0,62}):(([A-Za-z0-9-._~]|%[0-9A-F]{2})+)$/`)
)

type ChannelProps struct {
	Publisher      string
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

type Channel string

func (ch Channel) String() string {
	return string(ch)
}

func (ch Channel) ParseProvider() (string, string, string) {
	hashSplit := strings.Split(ch.String(), "#")
	name, extra := hashSplit[0], hashSplit[1]
	colonSplit := strings.Split(extra, ":")
	return name, colonSplit[0], colonSplit[1]
}

func (ch Channel) Normalize() *ChannelProps {
	found := providerRE.FindAllString(ch.String(), -1)
	if len(found) == 0 {
		return &ChannelProps{
			URL: ch.String(),
		}
	}
	name, suffix, value := ch.ParseProvider()
	return &ChannelProps{
		Publisher:      ch.String(),
		PublisherType:  "provider",
		ProviderName:   name,
		ProviderSuffix: suffix,
		ProviderValue:  value,
		TLD:            name + "#" + suffix,
		SLD:            ch.String(),
		RLD:            value,
	}
}
