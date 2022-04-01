package submitstatus

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

// checkCustodianSubmitResponse implement
func checkCustodianSubmitResponse(transactionStatus payment.TransactionStatus) (transactionstatus.State, error) {

	if transactionStatus.CustodianSubmissionResponse == nil || len(*transactionStatus.CustodianSubmissionResponse) == 0 {
		return transactionstatus.Unknown, fmt.Errorf("error custodian submission response empty for transaction with documentID %s",
			ptr.StringOr(transactionStatus.Transaction.DocumentID, "documentID is nil"))
	}

	switch ptr.String(transactionStatus.Transaction.Custodian) {
	case transactionstatus.Gemini:
		var payoutResult gemini.PayoutResult
		err := json.Unmarshal([]byte(*transactionStatus.CustodianSubmissionResponse), &payoutResult)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling gemini submit status response: %w", err)
		}
		return payoutResult.CheckStatus(), nil
	case transactionstatus.Uphold:
		var transactionResponse uphold.UpholdTransactionResponse
		err := json.Unmarshal([]byte(*transactionStatus.CustodianSubmissionResponse), &transactionResponse)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling uphold submit status response: %w", err)
		}
		return transactionResponse.CheckStatus(), nil
	case transactionstatus.Bitflyer:
		var withdrawToDepositIDResponse bitflyer.WithdrawToDepositIDResponse
		err := json.Unmarshal([]byte(*transactionStatus.CustodianSubmissionResponse), &withdrawToDepositIDResponse)
		if err != nil {
			return transactionstatus.Unknown, fmt.Errorf("error unmarshaling bitflyer submit status response: %w", err)
		}
		return withdrawToDepositIDResponse.CheckStatus(), nil
	}

	return transactionstatus.Unknown, fmt.Errorf("check custodian submit: error unknown custodian %s for transaction with documentID %s",
		ptr.StringOr(transactionStatus.Transaction.Custodian, "custodian is empty"),
		ptr.StringOr(transactionStatus.Transaction.DocumentID, "documentID is empty"))
}
