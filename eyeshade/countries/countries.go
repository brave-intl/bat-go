package countries

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/jsonutils"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	// OriginalRateID is the id for the original referral group
	OriginalRateID = uuid.FromStringOrNil("71341fc9-aeab-4766-acf0-d91d3ffb0bfa")
)

// Resolve resolves the many referral groups that come back
func Resolve(rows []ReferralGroup) *[]ReferralGroup {
	rowsFiltered := []ReferralGroup{}
	groupsByID := map[uuid.UUID]ReferralGroup{}
	filter := map[string]*ReferralGroup{}
	codesByID := map[uuid.UUID]jsonutils.JSONStringArray{}
	for _, group := range rows {
		groupsByID[group.ID] = group
		for _, country := range *group.Codes {
			if filter[country] != nil && group.ActiveAt.After((*filter[country]).ActiveAt) {
				continue
			}
			// set this value if it has not already been set
			// or if the current group is after the currently set group
			filter[country] = &group
		}
	}
	for country, group := range filter {
		codesByID[group.ID] = append(codesByID[group.ID], country)
	}
	for id, codes := range codesByID {
		codes := codes
		group := groupsByID[id]
		group.Codes = &codes
		rowsFiltered = append(rowsFiltered, group)
	}
	return &rowsFiltered
}

// FindGroup finds the matching group or reverts back to the original rate id
func FindGroup(passedGroupID uuid.UUID, groups []Group) Group {
	countryGroupID := passedGroupID
	if uuid.Equal(countryGroupID, uuid.Nil) {
		countryGroupID = OriginalRateID
	}
	group := FindByID(groups, countryGroupID)
	if group != nil {
		return *group
	}
	return *FindByID(groups, OriginalRateID)
}

// FindByID finds a referral group by its id
func FindByID(groups []Group, id uuid.UUID) *Group {
	for _, group := range groups {
		if uuid.Equal(group.ID, id) {
			return &group
		}
	}
	return nil
}

// ComputedValue holds computed information about a referral group
type ComputedValue struct {
	Probi    decimal.Decimal
	Value    decimal.Decimal
	Currency string
	ID       uuid.UUID
}

// Group holds information about a given referral group
type Group struct {
	ID       uuid.UUID       `json:"id" db:"id"`
	ActiveAt time.Time       `json:"activeAt" db:"active_at"`
	Name     string          `json:"name" db:"name"`
	Amount   decimal.Decimal `json:"amount" db:"amount"`
	Currency string          `json:"currency" db:"currency"`
}

// ReferralGroup holds information about a given referral group
type ReferralGroup struct {
	keys []string

	ID       uuid.UUID                  `json:"id" db:"id"`
	ActiveAt time.Time                  `json:"activeAt" db:"active_at"`
	Name     string                     `json:"name" db:"name"`
	Amount   decimal.Decimal            `json:"amount" db:"amount"`
	Currency string                     `json:"currency" db:"currency"`
	Codes    *jsonutils.JSONStringArray `json:"codes" db:"codes,omitempty"`
}

// MarshalJSON marshalles the referral group only including keys that were asked for +id
func (rg ReferralGroup) MarshalJSON() ([]byte, error) {
	keys := append([]string{"id"}, rg.keys...)
	keysHash := map[string]bool{}
	for _, key := range keys {
		keysHash[key] = true
	}
	params := []string{}
	val := reflect.ValueOf(rg).Elem()
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		if !keysHash[typeField.Name] {
			continue
		}
		jsonTag, ok := typeField.Tag.Lookup("json")
		if !ok {
			continue
		}
		split := strings.Split(jsonTag, ",")
		marshalled, err := json.Marshal(valueField.Interface())
		if err != nil {
			return nil, err
		}
		params = append(
			params,
			fmt.Sprintf(
				"%s:%s",
				strconv.Quote(split[0]),
				marshalled,
			),
		)
	}
	buffer := bytes.NewBufferString("{")
	buffer.WriteString(strings.Join(params, ","))
	buffer.WriteString("}")
	return buffer.Bytes(), nil
}

// SetKeys sets the keys property to allow for a subset of the group
// to be serialized during a json marshal
func (rg ReferralGroup) SetKeys(keys []string) {
	rg.keys = keys
}

// GroupByID creates a mapping of groups accessable by their id
func GroupByID(groups ...Group) map[string]Group {
	modifiers := map[string]Group{}
	for _, group := range groups {
		modifiers[group.ID.String()] = group
	}
	return modifiers
}

// CollectCurrencies collects the currencies from the
func CollectCurrencies(groups ...Group) []string {
	currencies := []string{}
	hash := map[string]bool{}
	for _, group := range groups {
		if hash[group.Currency] {
			continue
		}
		hash[group.Currency] = true
		currencies = append(currencies, group.Currency)
	}
	return currencies
}
