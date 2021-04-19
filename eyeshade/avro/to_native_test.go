// +build eyeshade

package avro

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type AvroNative struct {
	suite.Suite
}

func TestAvroNative(t *testing.T) {
	suite.Run(t, new(AvroNative))
}

func (suite *AvroNative) TestDeep() {
	type C struct {
		K1 int `json:"k1"`
	}
	type B struct {
		K1 int                     `json:"k1"`
		K2 uint                    `json:"k2"`
		K3 string                  `json:"k3"`
		K4 bool                    `json:"k4"`
		K5 time.Time               `json:"k5"`
		K6 decimal.Decimal         `json:"k6"`
		K7 []time.Time             `json:"k7"`
		K8 []C                     `json:"k8"`
		K9 altcurrency.AltCurrency `json:"k9"`
	}
	type A struct {
		K1 B `json:"k1"`
	}
	quarter := decimal.NewFromFloat(0.25)
	now := time.Now()
	s := A{
		K1: B{
			K1: -5,
			K2: 5,
			K3: "str",
			K4: true,
			K6: quarter,
			K7: []time.Time{
				now,
				now.Add(time.Second),
			},
			K8: []C{{
				K1: 1,
			}},
			K9: altcurrency.BAT,
		},
	}
	value := ToNative(s)
	b, err := json.Marshal(value)
	suite.Require().NoError(err)
	suite.Require().JSONEq(fmt.Sprintf(
		`{
		"k1": {
			"k1": -5,
			"k2": 5,
			"k3": "str",
			"k4": true,
			"k5": "0001-01-01T00:00:00Z",
			"k6": "%s",
			"k7": ["%s", "%s"],
			"k8": [{
				"k1": 1
			}],
			"k9": "BAT"
		}
	}`,
		quarter.String(),
		now.Format(time.RFC3339),
		now.Add(time.Second).Format(time.RFC3339),
	), string(string(b)))
}
