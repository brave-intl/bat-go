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
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	mockbitflyer "github.com/brave-intl/bat-go/utils/clients/bitflyer/mock"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type BitflyerMockSuite struct {
	suite.Suite
	client *mockbitflyer.MockClient
	token  string
	ctrl   *gomock.Controller
}

func (suite *BitflyerMockSuite) SetupSuite() {
	mockCtrl := gomock.NewController(suite.T())
	suite.token = os.Getenv("BITFLYER_TOKEN")
	suite.ctrl = mockCtrl
	suite.client = mockbitflyer.NewMockClient(mockCtrl)
}

func (suite *BitflyerMockSuite) SetupTest() {
}

func (suite *BitflyerMockSuite) TearDownSuite() {
	suite.ctrl.Finish()
}

func (suite *BitflyerMockSuite) TearDownTest() {
}

func (suite *BitflyerMockSuite) CleanDB() {
}

func TestBitflyerMockSuite(t *testing.T) {
	suite.Run(t, new(BitflyerMockSuite))
}

func (suite *BitflyerMockSuite) TestFailures() {
	ctx := context.Background()
	price := decimal.NewFromFloat(0.25)
	amount := decimal.NewFromFloat(2)
	settledAmountAsFloat, _ := amount.Sub(amount.Mul(decimal.NewFromFloat(0.05))).Float64()
	knownDepositID := uuid.NewV4()
	settlementTx0 := settlementTransaction(amount.String(), knownDepositID.String())
	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)
	sourceFrom := "tipping"

	tmpFile0 := suite.writeSettlementFiles([]settlement.Transaction{
		settlementTx0,
	})
	defer func() { _ = os.Remove(tmpFile0.Name()) }()

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*bitflyer.NewWithdrawToDepositIDBulkPayload(
				nil,
				priceToken.String(),
				&[]bitflyer.WithdrawToDepositIDPayload{{
					CurrencyCode: BAT,
					Amount:       settledAmountAsFloat,
					DepositID:    knownDepositID.String(),
					TransferID:   bitflyer.GenerateTransferID(&settlementTx0),
					SourceFrom:   sourceFrom,
				}},
			),
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: false,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       price,
				Status:       "NOT_FOUNTD",
				TransferID:   bitflyer.GenerateTransferID(&settlementTx0),
			}},
		}, nil)
	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile0.Name()},
		"tipping",
		nil,
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

	suite.client.EXPECT().SetAuthToken("")
	suite.client.SetAuthToken("")

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*bitflyer.NewWithdrawToDepositIDBulkPayload(
				nil,
				priceToken.String(),
				&[]bitflyer.WithdrawToDepositIDPayload{{
					CurrencyCode: BAT,
					Amount:       settledAmountAsFloat,
					DepositID:    knownDepositID.String(),
					TransferID:   bitflyer.GenerateTransferID(&settlementTx0),
					SourceFrom:   sourceFrom,
				}},
			),
		).
		Return(nil, clients.BitflyerError{
			Message:  uuid.NewV4().String(),
			ErrorIDs: []string{"1234"},
			Label:    "JsonError.TOKEN_ERROR",
			Status:   -1,
		})

	_, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile0.Name()},
		"tipping",
		nil, // dry run first
	)
	suite.client.EXPECT().SetAuthToken(suite.token)
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

func (suite *BitflyerMockSuite) TestFormData() {
	// suite.T().Skip("bitflyer side unable to settle")
	ctx := context.Background()
	address := "2492cdba-d33c-4a8d-ae5d-8799a81c61c2"
	sourceFrom := "tipping"
	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)
	price := decimal.NewFromFloat(0.25)
	amount := decimal.NewFromFloat(2)
	settledAmount := amount.Sub(amount.Mul(decimal.NewFromFloat(0.05)))
	settledAmountAsFloat, _ := settledAmount.Float64()
	duration, err := time.ParseDuration("4s")
	suite.Require().NoError(err)
	dryRunOptions := &bitflyer.DryRunOption{
		ProcessTimeSec: uint(duration.Seconds()),
	}

	settlementTx1 := settlementTransaction(amount.String(), address)
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

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*bitflyer.NewWithdrawToDepositIDBulkPayload(
				dryRunOptions,
				priceToken.String(),
				&[]bitflyer.WithdrawToDepositIDPayload{{
					CurrencyCode: BAT,
					Amount:       settledAmountAsFloat,
					DepositID:    address,
					TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
					SourceFrom:   sourceFrom,
				}},
			),
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: true,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       settledAmount,
				Status:       "SUCCESS",
				TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
			}},
		}, nil)

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

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*bitflyer.NewWithdrawToDepositIDBulkPayload(
				nil,
				priceToken.String(),
				&[]bitflyer.WithdrawToDepositIDPayload{{
					CurrencyCode: BAT,
					Amount:       settledAmountAsFloat,
					DepositID:    address,
					TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
					SourceFrom:   sourceFrom,
				}},
			),
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: true,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       settledAmount,
				Status:       "SUCCESS",
				TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
			}},
		}, nil)

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
		suite.client.EXPECT().
			FetchQuote(ctx, currencyCode).
			Return(&bitflyer.Quote{
				PriceToken:   priceToken.String(),
				ProductCode:  currencyCode,
				MainCurrency: "JPY",
				SubCurrency:  "BAT",
				Rate:         price,
			}, nil)

		suite.client.EXPECT().
			CheckPayoutStatus(
				ctx,
				*bitflyer.NewWithdrawToDepositIDBulkPayload(
					nil,
					priceToken.String(),
					&[]bitflyer.WithdrawToDepositIDPayload{{
						CurrencyCode: BAT,
						Amount:       settledAmountAsFloat,
						DepositID:    address,
						TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
						SourceFrom:   sourceFrom,
					}},
				),
			).
			Return(&bitflyer.WithdrawToDepositIDBulkResponse{
				DryRun: true,
				Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
					CurrencyCode: currencyCode,
					Amount:       settledAmount,
					Status:       "EXECUTED",
					TransferID:   bitflyer.GenerateTransferID(&settlementTx1),
				}},
			}, nil)

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
	three := decimal.NewFromFloat(3)
	settledAmount3 := three.Sub(three.Mul(decimal.NewFromFloat(0.05)))
	settledAmount3AsFloat, _ := settledAmount3.Float64()
	settlementTx2 := settlementTransaction(three.String(), address)
	settlementTx2.SettlementID = settlementTx1.SettlementID
	settlementTx2.Destination = settlementTx1.Destination
	settlementTx2.WalletProviderID = settlementTx1.WalletProviderID
	settlementTx2.ProviderID = bitflyer.GenerateTransferID(&settlementTx2) // add bitflyer transaction hash
	tmpFile2 := suite.writeSettlementFiles([]settlement.Transaction{
		settlementTx2,
	})
	defer func() { _ = os.Remove(tmpFile2.Name()) }()

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*bitflyer.NewWithdrawToDepositIDBulkPayload(
				nil,
				priceToken.String(),
				&[]bitflyer.WithdrawToDepositIDPayload{{
					CurrencyCode: BAT,
					Amount:       settledAmount3AsFloat,
					DepositID:    address,
					TransferID:   bitflyer.GenerateTransferID(&settlementTx2),
					SourceFrom:   sourceFrom,
				}},
			),
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: false,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       settledAmount3,
				Message:      "Duplicate transfer_id and different parameters",
				Status:       "OTHER_ERROR",
				TransferID:   bitflyer.GenerateTransferID(&settlementTx2),
			}},
		}, nil)

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

func (suite *BitflyerMockSuite) writeSettlementFiles(txs []settlement.Transaction) (filepath *os.File) {
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
