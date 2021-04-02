package eyeshade

import "strings"

type ChannelProps struct {
	Publisher      string
	PublisherType  string
	ProviderName   string
	ProviderSuffix string
	ProviderValue  string
	TLD            string
	SLD            string
	RLD            string
	QLD            string
}

type Channel string

func (ch Channel) String() string {
	return string(ch)
}

func (ch Channel) ParseProvider() (string, string) {
	hashSplit := strings.Split(ch.String(), "#")
	name, extra := hashSplit[0], hashSplit[1]
	colonSplit := strings.Split(extra, ":")
	return name, colonSplit[0]
}

func (ch Channel) Normalize() *ChannelProps {
	channelString := ch.String()
	found := providerRE.FindAllString(channelString, -1)
	if len(found) > 0 {
		providerName, providerSuffix := ch.ParseProvider()
		return &ChannelProps{
			ProviderName:   providerName,
			ProviderSuffix: providerSuffix,
		}
	}
	return &ChannelProps{
		ProviderName:   providerName,
		ProviderSuffix: providerSuffix,
	}
}
