package output

import (
	"reflect"

	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/eyeshade/models"
)

var (
	APIResponseTypes = []reflect.Type{
		reflect.TypeOf(models.AccountSettlementEarnings{}),
		reflect.TypeOf(models.AccountEarnings{}),
		reflect.TypeOf(models.Balance{}),
		reflect.TypeOf(countries.Group{}),
		reflect.TypeOf(countries.ReferralGroup{}),
		reflect.TypeOf(models.CreatorsTransaction{}),
	}
)
