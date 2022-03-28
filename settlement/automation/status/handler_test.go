package status

import (
	"fmt"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	"testing"
)

func TestStatus_Handle_TransactionStatus_Nil(t *testing.T) {
	var response interface{} = nil

	if transactionStatus, ok := response.(*payment.TransactionStatus); ok {
		fmt.Println(transactionStatus.Transaction)
	} else {
		fmt.Println("was nil")
	}

}
