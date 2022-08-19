package cmd

import (
	"reflect"

	"github.com/brave-intl/bat-go/services/rewards"
	"github.com/brave-intl/bat-go/services/wallet"
)

var (
	// APIResponseTypes - A list of all API response types used in bat-go services
	// primarily for auto generating the json-schema for each response type
	APIResponseTypes = []reflect.Type{
		reflect.TypeOf(wallet.ResponseV3{}),
		reflect.TypeOf(wallet.BalanceResponseV3{}),
		reflect.TypeOf(wallet.LinkBraveDepositAccountRequest{}),
		reflect.TypeOf(rewards.ParametersV1{}),
	}
)
