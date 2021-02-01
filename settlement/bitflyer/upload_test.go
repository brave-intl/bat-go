// +build integration

package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type BitflyerSuite struct {
	suite.Suite
	client bitflyer.Client
	token  string
}

func (suite *BitflyerSuite) SetupSuite() {
	// mockCtrl := gomock.NewController(suite.T())
	// defer mockCtrl.Finish()
	// suite.client = mockbitflyer.NewMockClient(mockCtrl)
	client, err := bitflyer.New()
	suite.client = client
	suite.Require().NoError(err)
	payload := bitflyer.TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          os.Getenv("BITFLYER_CLIENT_ID"),
		ClientSecret:      os.Getenv("BITFLYER_CLIENT_SECRET"),
		ExtraClientSecret: os.Getenv("BITFLYER_EXTRA_CLIENT_SECRET"),
	}
	auth, err := client.RefreshToken(
		context.Background(),
		payload,
	)
	suite.Require().NoError(err)
	suite.token = auth.AccessToken
	client.SetAuthToken(auth.AccessToken)
}

func (suite *BitflyerSuite) SetupTest() {
}

func (suite *BitflyerSuite) TearDownTest() {
}

func (suite *BitflyerSuite) CleanDB() {
}

func TestBitflyerSuite(t *testing.T) {
	suite.Run(t, new(BitflyerSuite))
}

func settlementTransaction(amount, address string) settlement.Transaction {
	amountDecimal, _ := decimal.NewFromString(amount)
	bat := altcurrency.BAT
	feeFactor := decimal.NewFromFloat32(0.05)
	fees := amountDecimal.Mul(feeFactor)
	settlementID := uuid.NewV4().String()
	return settlement.Transaction{
		AltCurrency:      &bat,
		Currency:         "BAT",
		Amount:           amountDecimal,
		Probi:            amountDecimal.Sub(fees).Mul(decimal.New(1, 18)),
		BATPlatformFee:   fees.Mul(decimal.New(1, 18)),
		Destination:      address,
		SettlementID:     settlementID,
		WalletProvider:   "bitflyer",
		WalletProviderID: uuid.NewV4().String(),
		TransferFee:      decimal.NewFromFloat(0),
		ExchangeFee:      decimal.NewFromFloat(0),
		ProviderID: bitflyer.GenerateTransferID(&settlement.Transaction{
			SettlementID: settlementID,
			Destination:  address,
		}),
	}
}

func transactionSubmitted(status string, tx settlement.Transaction, note string) settlement.Transaction {
	return settlement.Transaction{
		Status:           status,
		AltCurrency:      tx.AltCurrency,
		Currency:         tx.Currency,
		ProviderID:       tx.ProviderID,
		Amount:           tx.Amount,
		Probi:            tx.Probi,
		BATPlatformFee:   tx.BATPlatformFee,
		Destination:      tx.Destination,
		SettlementID:     tx.SettlementID,
		WalletProvider:   tx.WalletProvider,
		WalletProviderID: tx.WalletProviderID,
		ExchangeFee:      tx.ExchangeFee,
		TransferFee:      tx.TransferFee,
		Note:             note,
	}
}

func (suite *BitflyerSuite) TestFailures() {
	ctx := context.Background()
	settlementTx0 := settlementTransaction("2", uuid.NewV4().String())
	tmpFile0 := suite.writeSettlementFiles([]settlement.Transaction{
		settlementTx0,
	})
	defer func() { _ = os.Remove(tmpFile0.Name()) }()

	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile0.Name()},
		"self",
		nil, // dry run first
	)
	suite.Require().NoError(err)
	completeTxs := (*payoutFiles)["complete"]
	suite.Require().Len(completeTxs, 0, "one tx complete")
	failedTxs := (*payoutFiles)["failed"]
	suite.Require().Len(failedTxs, 1, "one tx failed")

	failedBytes, err := json.Marshal(failedTxs)
	settlementTx0.ProviderID = bitflyer.GenerateTransferID(&settlementTx0)
	failedTxNote := failedTxs[0].Note
	suite.Require().True(strings.Contains(failedTxNote, "NOT_FOUNTD"))
	expectedBytes, err := json.Marshal([]settlement.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("failed", settlementTx0, failedTxNote),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(expectedBytes),
		string(failedBytes),
		"dry runs only pass through validation currently",
	)

	suite.client.SetAuthToken("")
	_, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile0.Name()},
		"self",
		nil, // dry run first
	)
	suite.client.SetAuthToken(suite.token)
	suite.Require().Error(err)
	bfErr, ok := err.(clients.BitflyerError)
	suite.Require().True(ok)
	errSerialized, err := json.Marshal(bfErr)
	suite.Require().JSONEq(
		fmt.Sprintf(`{
			"message": "%s",
			"label": "JsonError.TOKEN_ERROR",
			"status": -1,
			"errors": ["%s"]
		}`, bfErr.Message, bfErr.ErrorIDs[0]),
		string(errSerialized),
	)
}

func (suite *BitflyerSuite) TestFormData() {
	ctx := context.Background()
	address := "2492cdba-d33c-4a8d-ae5d-8799a81c61c2"
	sourceFrom := "self"
	dryRunOptions := &bitflyer.DryRunOption{
		ProcessTimeSec: 4,
	}

	settlementTx1 := settlementTransaction("2", address)
	tmpFile1 := suite.writeSettlementFiles([]settlement.Transaction{
		settlementTx1,
	})
	defer func() { _ = os.Remove(tmpFile1.Name()) }()
	/*
		resultIteration := make(map[string]int)

		var payoutFiles *map[string][]settlement.Transaction
		for i := 0; i < 2; i++ {
			<-time.After(2 * time.Second)
			results, err := IterateRequest(
				ctx,
				"upload",
				suite.client,
				[]string{tmpFile1.Name()},
				sourceFrom,
				dryRunOptions, // dry run first
			)
			suite.Require().NoError(err)
			for key, items := range *results {
				resultIteration[key] += len(items)
			}
			payoutFiles = results
		}
		suite.Require().Equal(map[string]int{
			"pending":  1,
			"complete": 1,
		}, resultIteration)
	*/

	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile1.Name()},
		sourceFrom,
		dryRunOptions, // dry run first
	)
	suite.Require().NoError(err)
	completedDryRunTxs := (*payoutFiles)["complete"]
	suite.Require().Len(completedDryRunTxs, 1, "one transaction should be created")

	completedDryRunBytes, err := json.Marshal(completedDryRunTxs)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = bitflyer.GenerateTransferID(&settlementTx1)
	expectedBytes, err := json.Marshal([]settlement.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS"),
	})
	suite.Require().JSONEq(
		string(completedDryRunBytes),
		string(expectedBytes),
		"dry runs only pass through validation currently",
	)
	dryRunOptions.ProcessTimeSec = 0

	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile1.Name()},
		sourceFrom,
		nil,
	)
	suite.Require().NoError(err)
	// setting an array on the "complete" key means we will have a file written
	// with the suffix of "complete" when this function is called in the cli scripts
	completed := (*payoutFiles)["complete"]
	suite.Require().Len(completed, 1, "one transaction should be created")
	completeSerialized, err := json.Marshal(completed)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = bitflyer.GenerateTransferID(&settlementTx1) // add bitflyer transaction hash
	mCompleted, err := json.Marshal([]settlement.Transaction{              // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(completeSerialized),
		string(mCompleted),
	)

	var completedStatus []settlement.Transaction
	for {
		<-time.After(time.Second)
		payoutFiles, err = IterateRequest(
			ctx,
			"checkstatus",
			suite.client,
			[]string{tmpFile1.Name()},
			sourceFrom,
			nil,
		)
		suite.Require().NoError(err)
		completedStatus = (*payoutFiles)["complete"]
		// useful if the loop never finishes
		// fmt.Printf("checkstatus %#v\n", *payoutFiles)
		if len(completedStatus) > 0 {
			break
		}
	}
	suite.Require().Len(completedStatus, 1, "one transaction should be created")
	completeSerializedStatus, err := json.Marshal(completedStatus)
	suite.Require().NoError(err)

	mCompletedStatus, err := json.Marshal([]settlement.Transaction{
		transactionSubmitted("complete", settlementTx1, "EXECUTED"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(completeSerializedStatus), string(mCompletedStatus))

	// make a new tx that will conflict with previous
	settlementTx2 := settlementTransaction("3", address)
	settlementTx2.SettlementID = settlementTx1.SettlementID
	settlementTx2.Destination = settlementTx1.Destination
	settlementTx2.WalletProviderID = settlementTx1.WalletProviderID
	settlementTx2.ProviderID = bitflyer.GenerateTransferID(&settlementTx2) // add bitflyer transaction hash
	tmpFile2 := suite.writeSettlementFiles([]settlement.Transaction{
		settlementTx2,
	})
	defer func() { _ = os.Remove(tmpFile2.Name()) }()
	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile2.Name()},
		sourceFrom,
		nil,
	)
	suite.Require().NoError(err)
	idempotencyFailComplete := (*payoutFiles)["complete"]
	idempotencyFailInvalidInput := (*payoutFiles)["failed"]
	suite.Require().Len(idempotencyFailComplete, 0, "one transaction should be created")
	suite.Require().Len(idempotencyFailInvalidInput, 1, "one transaction should have malformed amount")
	idempotencyFailInvalidInputActual, err := json.Marshal(idempotencyFailInvalidInput)
	suite.Require().NoError(err)

	// bitflyer does not send us back what we sent it
	// so we end up in an odd space if we change amount or other inputs
	// which is ok, it just makes for odd checks
	// in this particular case, we return the original transactions with an "failed" status
	// which is why we do not need to modify the number amounts
	//
	// the invalid-input part is what will put the transaction in a different file
	// so that we do not send to eyeshade
	idempotencyFailNote := idempotencyFailInvalidInput[0].Note
	suite.Require().Equal("OTHER_ERROR: Duplicate transfer_id and different parameters", idempotencyFailNote)
	idempotencyFailInvalidInputExpected, err := json.Marshal([]settlement.Transaction{
		transactionSubmitted("failed", settlementTx2, idempotencyFailNote),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(idempotencyFailInvalidInputExpected),
		string(idempotencyFailInvalidInputActual),
	)
}

func (suite *BitflyerSuite) writeSettlementFiles(txs []settlement.Transaction) (filepath *os.File) {
	tmpDir := os.TempDir()
	tmpFile, err := ioutil.TempFile(tmpDir, "bat-go-test-bitflyer-upload-")
	suite.Require().NoError(err)

	json, err := json.Marshal(txs)
	suite.Require().NoError(err)

	_, err = tmpFile.Write([]byte(json))
	suite.Require().NoError(err)
	err = tmpFile.Close()
	suite.Require().NoError(err)
	return tmpFile
}
