package eyeshade

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/utils/altcurrency"
)

var (
	// Modifiers a map of modifiers for avro decoders
	Modifiers = map[string]func(*MessageHandler) ([]map[string]string, error){
		avro.KeyToTopic["referral"]: ModifierReferral,
	}
)

// ModifierReferral holds the information needed to modify referral messages correctly
func ModifierReferral(con *MessageHandler) ([]map[string]string, error) {
	def := []map[string]string{}
	groups, err := con.service.Datastore(true).
		GetActiveCountryGroups(con.Context())
	if err != nil {
		return def, err
	}
	modifiers := map[string]string{}
	currencies := []string{}
	for _, group := range *groups {
		currencies = append(currencies, group.Currency)
	}
	rates, err := con.service.Clients.Ratios.FetchRate(
		con.Context(),
		altcurrency.BAT.String(),
		currencies...,
	)
	if err != nil {
		return def, err
	}
	for _, group := range *groups {
		computed := countries.ComputeValue(
			group,
			rates.Payload,
		)
		modifiers[group.ID.String()] = computed.Probi.String()
	}
	return append(def, modifiers), nil
}
