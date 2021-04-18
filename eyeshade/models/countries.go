package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
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
func Resolve(groups []ReferralGroup) *[]ReferralGroup {
	rowsFiltered := []ReferralGroup{}
	groupsByID := map[string]ReferralGroup{}
	countryToGoup := map[string]*ReferralGroup{}
	groupIDToCodesList := map[string][]string{}
	for i := range groups {
		group := groups[i]
		groupsByID[group.ID.String()] = group
		for _, country := range group.Codes {
			if countryToGoup[country] != nil {
				if group.ActiveAt.After((*countryToGoup[country]).ActiveAt) {
					countryToGoup[country] = &group
				}
				continue
			}
			// set this value if it has not already been set
			// or if the current group is after the currently set group
			countryToGoup[country] = &group
		}
	}
	for country, group := range countryToGoup {
		id := group.ID.String()
		groupIDToCodesList[id] = append(groupIDToCodesList[id], country)
	}
	groupIDs := []string{}
	for id := range groupIDToCodesList {
		groupIDs = append(groupIDs, id)
	}
	sort.Strings(groupIDs)
	for _, groupID := range groupIDs {
		codes := groupIDToCodesList[groupID]
		group := groupsByID[groupID]
		sort.Strings(codes)
		group.Codes = jsonutils.JSONStringArray(codes)
		rowsFiltered = append(rowsFiltered, group)
	}
	return &rowsFiltered
}

// FindReferralGroup finds the matching group or reverts back to the original rate id
func FindReferralGroup(passedGroupID uuid.UUID, groups []ReferralGroup) ReferralGroup {
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
func FindByID(groups []ReferralGroup, id uuid.UUID) *ReferralGroup {
	for _, group := range groups {
		if uuid.Equal(group.ID, id) {
			return &group
		}
	}
	return nil
}

// ReferralGroup holds information about a given referral group
type ReferralGroup struct {
	keys []string

	ID       uuid.UUID                 `json:"id" db:"id"`
	ActiveAt time.Time                 `json:"activeAt" db:"active_at"`
	Name     string                    `json:"name" db:"name"`
	Amount   decimal.Decimal           `json:"amount" db:"amount"`
	Currency string                    `json:"currency" db:"currency"`
	Codes    jsonutils.JSONStringArray `json:"codes" db:"codes,omitempty"`
}

// MarshalJSON marshalles the referral group only including keys that were asked for +id
func (rg ReferralGroup) MarshalJSON() ([]byte, error) {
	keys := append([]string{"id"}, rg.keys...)
	keysHash := map[string]bool{}
	sort.Strings(keys)
	for _, key := range keys {
		keysHash[key] = true
	}
	params := []string{}
	val := reflect.ValueOf(rg)
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		jsonTag, ok := typeField.Tag.Lookup("json")
		if !ok {
			continue
		}
		split := strings.Split(jsonTag, ",")
		key := split[0]
		if !keysHash[key] {
			continue
		}
		if key == "codes" && valueField.IsNil() {
			valueField = reflect.ValueOf([]string{})
		}
		marshalled, err := json.Marshal(valueField.Interface())
		if err != nil {
			return nil, err
		}
		params = append(
			params,
			fmt.Sprintf(
				"%s:%s",
				strconv.Quote(key),
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
func (rg ReferralGroup) SetKeys(keys []string) ReferralGroup {
	rg.keys = keys
	return rg
}

// ReferralGroupByID creates a mapping of groups accessable by their id
func ReferralGroupByID(groups ...ReferralGroup) map[string]ReferralGroup {
	modifiers := map[string]ReferralGroup{}
	for _, group := range groups {
		modifiers[group.ID.String()] = group
	}
	return modifiers
}

// CollectCurrencies collects the currencies from the
func CollectCurrencies(groups ...ReferralGroup) []string {
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
