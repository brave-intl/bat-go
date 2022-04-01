package checkstatus

import (
	"encoding/json"
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
)

// checkCustodianStatusResponse implement
func checkCustodianStatusResponse(transactionStatus payment.TransactionStatus) (transactionstatus.State, error) {
	if transactionStatus.CustodianStatusResponse == nil || len(*transactionStatus.CustodianStatusResponse) == 0 {
		return transactionstatus.Unknown, fmt.Errorf("error custodian status response empty for transaction with documentID %s",
			ptr.StringOr(transactionStatus.Transaction.DocumentID, "documentID is nil"))
	}

	switch ptr.String(transactionStatus.Transaction.Custodian) {
	case transactionstatus.Gemini:
		var payoutResult gemini.PayoutResult
		err := json.Unmarshal([]byte(*transactionStatus.CustodianStatusResponse), &payoutResult)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling gemini status response: %w", err)
		}
		return payoutResult.CheckStatus(), nil
	case transactionstatus.Uphold:
		var transactionResponse uphold.UpholdTransactionResponse
		err := json.Unmarshal([]byte(*transactionStatus.CustodianStatusResponse), &transactionResponse)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling uphold status response: %w", err)
		}
		return transactionResponse.CheckStatus(), nil
	case transactionstatus.Bitflyer:
		var withdrawToDepositIDResponse bitflyer.WithdrawToDepositIDResponse
		err := json.Unmarshal([]byte(*transactionStatus.CustodianStatusResponse), &withdrawToDepositIDResponse)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling bitflyer status response: %w", err)
		}
		return withdrawToDepositIDResponse.CheckStatus(), nil
	}

	return transactionstatus.Unknown, fmt.Errorf("check custodian status: error unknown custodian %s for transaction with documentID %s",
		ptr.StringOr(transactionStatus.Transaction.Custodian, "custodian is empty"),
		ptr.StringOr(transactionStatus.Transaction.DocumentID, "documentID is empty"))
}
