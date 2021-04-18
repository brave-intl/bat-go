package output

import (
	"reflect"

	"github.com/brave-intl/bat-go/eyeshade/models"
)

var (
	APIResponseTypes = []reflect.Type{
		reflect.TypeOf(models.AccountSettlementEarnings{}),
		reflect.TypeOf(models.AccountEarnings{}),
		reflect.TypeOf(models.Balance{}),
		reflect.TypeOf(models.ReferralGroup{}),
		reflect.TypeOf(models.CreatorsTransaction{}),
	}
)
