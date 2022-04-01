package submitstatus

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	"github.com/brave-intl/bat-go/utils/ptr"
	testutils "github.com/brave-intl/bat-go/utils/test"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCustodianSubmitResponse_Nil(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: nil,
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.EqualError(t, err, fmt.Sprintf("error custodian submission response empty for transaction with documentID %s", documentID))
}

func TestCheckCustodianSubmitResponse_Empty(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(""),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.EqualError(t, err, fmt.Sprintf("error custodian submission response empty for transaction with documentID %s", documentID))
}

func TestCheckCustodianStatusResponse_Gemini(t *testing.T) {
	documentID := testutils.RandomString()

	payoutResult, err := json.Marshal(gemini.PayoutResult{
		Status: ptr.FromString("completed"),
	})
	require.NoError(t, err)

	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Gemini),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(string(payoutResult)),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Complete, status)
	assert.NoError(t, err)
}

func TestCheckCustodianStatusResponse_Gemini_Error(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Gemini),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(testutils.RandomString()),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.Contains(t, err.Error(), "error unmarshaling gemini submit status response")
}

func TestCheckCustodianSubmitResponse_Uphold(t *testing.T) {
	documentID := testutils.RandomString()

	transactionResponse, err := json.Marshal(uphold.UpholdTransactionResponse{
		Status: "completed",
	})
	require.NoError(t, err)

	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Uphold),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(string(transactionResponse)),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Complete, status)
	assert.NoError(t, err)
}

func TestCheckCustodianSubmitResponse_Uphold_Error(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Uphold),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(testutils.RandomString()),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.Contains(t, err.Error(), "error unmarshaling uphold submit status response")
}

func TestCheckCustodianSubmitResponse_Bitflyer(t *testing.T) {
	documentID := testutils.RandomString()

	withdrawToDepositIDResponse, err := json.Marshal(bitflyer.WithdrawToDepositIDResponse{
		Status: "SUCCESS",
	})
	require.NoError(t, err)

	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Bitflyer),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(string(withdrawToDepositIDResponse)),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Complete, status)
	assert.NoError(t, err)
}

func TestCheckCustodianSubmitResponse_Bitflyer_Error(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(transactionstatus.Bitflyer),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(testutils.RandomString()),
	}

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.Contains(t, err.Error(), "error unmarshaling bitflyer submit status response:")
}

func TestCheckCustodianStatusResponse_UnknownCustodian(t *testing.T) {
	documentID := testutils.RandomString()
	transactionStatus := payment.TransactionStatus{
		Transaction: payment.Transaction{
			Custodian:  ptr.FromString(testutils.RandomString()),
			DocumentID: ptr.FromString(documentID),
		},
		CustodianSubmissionResponse: ptr.FromString(testutils.RandomString()),
	}

	expected := fmt.Sprintf("check custodian submit: error unknown custodian %s for transaction with documentID %s",
		*transactionStatus.Transaction.Custodian, *transactionStatus.Transaction.DocumentID)

	status, err := checkCustodianSubmitResponse(transactionStatus)

	assert.Equal(t, transactionstatus.Unknown, status)
	assert.EqualError(t, err, expected)
}
