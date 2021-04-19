// +build eyeshade

package models

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type CountriesSuite struct {
	suite.Suite
}

func TestCountriesSuite(t *testing.T) {
	suite.Run(t, new(CountriesSuite))
}

func (suite *CountriesSuite) TestGroupMarshal() {
	type Scenario struct {
		name   string
		input  models.ReferralGroup
		expect string
	}
	rawInput := models.ReferralGroup{}
	rawInputWithCodesField := models.ReferralGroup{}.SetKeys([]string{"codes"})
	rawInputWithCodesValues := models.ReferralGroup{}.SetKeys([]string{"codes"})
	rawInputWithCodesValues.Codes = pq.StringArray([]string{"CA", "US"})

	scenarios := []Scenario{{
		name:  "raw",
		input: rawInput,
		expect: fmt.Sprintf(`{
			"id": "%s"
		}`, uuid.UUID{}.String()),
	}, {
		name:  "with codes",
		input: rawInputWithCodesField,
		expect: fmt.Sprintf(`{
			"id": "%s",
			"codes": []
		}`, uuid.UUID{}.String()),
	}, {
		name:  "with values",
		input: rawInputWithCodesValues,
		expect: fmt.Sprintf(`{
			"id": "%s",
			"codes": ["CA", "US"]
		}`, uuid.UUID{}.String()),
	}}
	for _, scenario := range scenarios {
		suite.Run(scenario.name, func() {
			suite.Require().JSONEq(
				scenario.expect,
				must.Marshal(suite.Require(), scenario.input),
			)
		})
	}
}

func (suite *CountriesSuite) TestGroupResolve() {
	type Scenario struct {
		name   string
		input  []models.ReferralGroup
		expect []models.ReferralGroup
	}
	stringIDs := must.UUIDsToString(must.RandomIDs(4)...)
	sort.Strings(stringIDs)
	ids := []uuid.UUID{}
	for _, id := range stringIDs {
		ids = append(ids, uuid.Must(uuid.FromString(id)))
	}
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]
	now0 := time.Now()
	now1 := now0.Add(time.Second)
	now2 := now1.Add(time.Second)
	scenarios := []Scenario{{
		name: "condense",
		input: []models.ReferralGroup{
			{ID: a, ActiveAt: now0, Codes: []string{"us", "uk", "fr"}},
			{ID: b, ActiveAt: now1, Codes: []string{"uk", "jp"}},
			{ID: c, ActiveAt: now1, Codes: []string{"us", "de"}},
			{ID: d, ActiveAt: now2, Codes: []string{"jp"}},
		},
		expect: []models.ReferralGroup{
			{ID: a, ActiveAt: now0, Codes: []string{"fr"}},
			{ID: b, ActiveAt: now1, Codes: []string{"uk"}},
			{ID: c, ActiveAt: now1, Codes: []string{"de", "us"}},
			{ID: d, ActiveAt: now2, Codes: []string{"jp"}},
		},
	}, {
		name: "new will win",
		input: []models.ReferralGroup{
			{ID: a, ActiveAt: now0, Codes: []string{"us", "uk", "fr"}},
			{ID: b, ActiveAt: now1, Codes: []string{"uk"}},
		},
		expect: []models.ReferralGroup{
			{ID: a, ActiveAt: now0, Codes: []string{"fr", "us"}},
			{ID: b, ActiveAt: now1, Codes: []string{"uk"}},
		},
	}, {
		name: "old will not",
		input: []models.ReferralGroup{
			{ID: a, ActiveAt: now1, Codes: []string{"us", "uk", "fr"}},
			{ID: b, ActiveAt: now0, Codes: []string{"uk"}},
		},
		expect: []models.ReferralGroup{
			{ID: a, ActiveAt: now1, Codes: []string{"fr", "uk", "us"}},
		},
	}}
	for _, scenario := range scenarios {
		suite.Run(scenario.name, func() {
			keys := []string{"id", "activeAt", "codes"}
			for i := range scenario.input {
				scenario.input[i] = scenario.input[i].SetKeys(keys)
			}
			for i := range scenario.expect {
				scenario.expect[i] = scenario.expect[i].SetKeys(keys)
			}
			actual := models.Resolve(scenario.input)
			suite.Require().JSONEq(
				must.Marshal(suite.Require(), scenario.expect),
				must.Marshal(suite.Require(), actual),
			)
		})
	}
}
