//go:build integration
// +build integration

package bitflyersettlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	mockbitflyer "github.com/brave-intl/bat-go/libs/clients/bitflyer/mock"
	"github.com/brave-intl/bat-go/libs/custodian"
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
	amount := decimal.NewFromFloat(1.9)
	amountAsFloat, _ := amount.Float64()
	settlementTx0 := settlementTransaction(amount.String(), uuid.NewV4().String())
	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)
	sourceFrom := "tipping"

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	preparedTransactions, err := PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx0},
		false,
		"tipping",
	)

	suite.Require().NoError(err)

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	suite.client.EXPECT().
		CheckInventory(ctx).
		Return(map[string]bitflyer.Inventory{
			"BAT": {
				CurrencyCode: "BAT",
				Amount:       decimal.NewFromFloat(4.1),
				Available:    decimal.NewFromFloat(4.1),
			},
		}, nil)
	suite.client.EXPECT().
		CheckInventory(ctx).
		Return(map[string]bitflyer.Inventory{
			"BAT": {
				CurrencyCode: "BAT",
				Amount:       decimal.NewFromFloat(2.2),
				Available:    decimal.NewFromFloat(2.2),
			},
		}, nil)

	withdrawToDepositIDBulkPayload := bitflyer.NewWithdrawToDepositIDBulkPayload(
		nil,
		priceToken.String(),
		&[]bitflyer.WithdrawToDepositIDPayload{{
			CurrencyCode: BAT,
			Amount:       amountAsFloat,
			DepositID:    settlementTx0.Destination,
			TransferID:   settlementTx0.BitflyerTransferID(),
			SourceFrom:   sourceFrom,
		}},
	)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*withdrawToDepositIDBulkPayload,
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: false,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       price,
				Status:       "NOT_FOUND",
				TransferID:   settlementTx0.BitflyerTransferID(),
			}},
		}, nil)
	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil,
	)
	suite.Require().NoError(err)
	completeTxs := payoutFiles["complete"]
	suite.Require().Len(completeTxs, 0, "one tx complete")
	failedTxs := payoutFiles["failed"]
	suite.Require().Len(failedTxs, 1, "one tx failed")

	failedBytes, err := json.Marshal(failedTxs)
	settlementTx0.ProviderID = settlementTx0.TransferID()
	failedTxNote := failedTxs[0].Note
	suite.Require().True(strings.Contains(failedTxNote, "NOT_FOUND"))
	expectedBytes, err := json.Marshal([]custodian.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
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
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	withdrawToDepositIDBulkPayload = bitflyer.NewWithdrawToDepositIDBulkPayload(
		nil,
		priceToken.String(),
		&[]bitflyer.WithdrawToDepositIDPayload{{
			CurrencyCode: BAT,
			Amount:       amountAsFloat,
			DepositID:    settlementTx0.Destination,
			TransferID:   settlementTx0.BitflyerTransferID(),
			SourceFrom:   sourceFrom,
		}},
	)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*withdrawToDepositIDBulkPayload,
		).
		Return(nil, &clients.BitflyerError{
			Message:  uuid.NewV4().String(),
			ErrorIDs: []string{"1234"},
			Label:    "JsonError.TOKEN_ERROR",
			Status:   -1,
		})

	_, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil, // dry run first
	)
	suite.client.EXPECT().SetAuthToken(suite.token)
	suite.client.SetAuthToken(suite.token)
	suite.Require().Error(err)

	var bfErr *clients.BitflyerError
	ok := errors.As(err, &bfErr)
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
	amount := decimal.NewFromFloat(1.9)
	amountAsFloat, _ := amount.Float64()
	duration, err := time.ParseDuration("4s")
	suite.Require().NoError(err)
	dryRunOptions := &bitflyer.DryRunOption{
		ProcessTimeSec: uint(duration.Seconds()),
	}

	settlementTx1 := settlementTransaction(amount.String(), address)

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	preparedTransactions, err := PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx1},
		false,
		sourceFrom,
	)
	suite.Require().NoError(err)

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	suite.client.EXPECT().
		CheckInventory(ctx).
		Return(map[string]bitflyer.Inventory{
			"BAT": {
				CurrencyCode: "BAT",
				Amount:       decimal.NewFromFloat(4.1),
				Available:    decimal.NewFromFloat(4.1),
			},
		}, nil)
	suite.client.EXPECT().
		CheckInventory(ctx).
		Return(map[string]bitflyer.Inventory{
			"BAT": {
				CurrencyCode: "BAT",
				Amount:       decimal.NewFromFloat(2.2),
				Available:    decimal.NewFromFloat(2.2),
			},
		}, nil)

	withdrawToDepositIDBulkPayload := bitflyer.NewWithdrawToDepositIDBulkPayload(
		dryRunOptions,
		priceToken.String(),
		&[]bitflyer.WithdrawToDepositIDPayload{{
			CurrencyCode: BAT,
			Amount:       amountAsFloat,
			DepositID:    address,
			TransferID:   settlementTx1.BitflyerTransferID(),
			SourceFrom:   sourceFrom,
		}},
	)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*withdrawToDepositIDBulkPayload,
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: true,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       amount,
				Status:       "SUCCESS",
				TransferID:   settlementTx1.BitflyerTransferID(),
			}},
		}, nil)

	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		dryRunOptions, // dry run first
	)

	suite.Require().NoError(err)
	completedDryRunTxs := payoutFiles["complete"]
	suite.Require().Len(completedDryRunTxs, 1, "one transaction should be created")

	completedDryRunBytes, err := json.Marshal(completedDryRunTxs)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = settlementTx1.TransferID()
	expectedBytes, err := json.Marshal([]custodian.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS transferID: "+settlementTx1.BitflyerTransferID()),
	})
	suite.Require().JSONEq(
		string(expectedBytes),
		string(completedDryRunBytes),
		"dry runs only pass through validation currently",
	)
	dryRunOptions.ProcessTimeSec = 0

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	withdrawToDepositIDBulkPayload = bitflyer.NewWithdrawToDepositIDBulkPayload(
		nil,
		priceToken.String(),
		&[]bitflyer.WithdrawToDepositIDPayload{{
			CurrencyCode: BAT,
			Amount:       amountAsFloat,
			DepositID:    address,
			TransferID:   settlementTx1.BitflyerTransferID(),
			SourceFrom:   sourceFrom,
		}},
	)
	suite.client.EXPECT().
		CheckInventory(ctx).
		Return(map[string]bitflyer.Inventory{
			"BAT": {
				CurrencyCode: "BAT",
				Amount:       decimal.NewFromFloat(3.2),
				Available:    decimal.NewFromFloat(3.2),
			},
		}, nil)

	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*withdrawToDepositIDBulkPayload,
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: true,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       amount,
				Status:       "SUCCESS",
				TransferID:   settlementTx1.BitflyerTransferID(),
			}},
		}, nil)

	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil,
	)
	suite.Require().NoError(err)
	// setting an array on the "complete" key means we will have a file written
	// with the suffix of "complete" when this function is called in the cli scripts
	completed := payoutFiles["complete"]
	suite.Require().Len(completed, 1, "one transaction should be created")
	completeSerialized, err := json.Marshal(completed)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = settlementTx1.TransferID()    // add bitflyer transaction hash
	mCompleted, err := json.Marshal([]custodian.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS transferID: "+settlementTx1.BitflyerTransferID()),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(completeSerialized),
		string(mCompleted),
	)
	var completedStatus []custodian.Transaction
	for {
		<-time.After(time.Second)
		suite.client.EXPECT().
			FetchQuote(ctx, currencyCode, true).
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
				bitflyer.NewWithdrawToDepositIDBulkPayload(
					nil,
					priceToken.String(),
					&[]bitflyer.WithdrawToDepositIDPayload{{
						CurrencyCode: BAT,
						Amount:       amountAsFloat,
						DepositID:    address,
						TransferID:   settlementTx1.BitflyerTransferID(),
						SourceFrom:   sourceFrom,
					}},
				).ToBulkStatus(),
			).
			Return(&bitflyer.WithdrawToDepositIDBulkResponse{
				DryRun: true,
				Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
					Status:     "EXECUTED",
					TransferID: settlementTx1.BitflyerTransferID(),
				}},
			}, nil)

		payoutFiles, err = IterateRequest(
			ctx,
			"checkstatus",
			suite.client,
			*preparedTransactions,
			nil,
		)
		suite.Require().NoError(err)
		completedStatus = payoutFiles["complete"]
		// useful if the loop never finishes
		if len(completedStatus) > 0 {
			break
		}
	}
	suite.Require().Len(completedStatus, 1, "one transaction should be created")
	completeSerializedStatus, err := json.Marshal(completedStatus)
	suite.Require().NoError(err)

	mCompletedStatus, err := json.Marshal([]custodian.Transaction{
		transactionSubmitted("complete", settlementTx1, "EXECUTED transferID: "+settlementTx1.BitflyerTransferID()),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(mCompletedStatus), string(completeSerializedStatus))

	// make a new tx that will conflict with previous
	three := decimal.NewFromFloat(2.85)
	threeAsFloat, _ := three.Float64()
	settlementTx2 := settlementTransaction(three.String(), address)
	settlementTx2.SettlementID = settlementTx1.SettlementID
	settlementTx2.Destination = settlementTx1.Destination
	settlementTx2.WalletProviderID = settlementTx1.WalletProviderID
	settlementTx2.ProviderID = settlementTx2.BitflyerTransferID() // add bitflyer transaction hash

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	preparedTransactions, err = PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx2},
		false,
		sourceFrom,
	)

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)
	withdrawToDepositIDBulkPayload = bitflyer.NewWithdrawToDepositIDBulkPayload(
		nil,
		priceToken.String(),
		&[]bitflyer.WithdrawToDepositIDPayload{{
			CurrencyCode: BAT,
			Amount:       threeAsFloat,
			DepositID:    address,
			TransferID:   settlementTx2.BitflyerTransferID(),
			SourceFrom:   sourceFrom,
		}},
	)
	suite.client.EXPECT().
		UploadBulkPayout(
			ctx,
			*withdrawToDepositIDBulkPayload,
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: false,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: currencyCode,
				Amount:       amount,
				Message:      "Duplicate transfer_id and different parameters",
				Status:       "OTHER_ERROR",
				TransferID:   settlementTx2.BitflyerTransferID(),
			}},
		}, nil)

	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil,
	)
	suite.Require().NoError(err)
	idempotencyFailComplete := payoutFiles["failed"]
	suite.Require().Len(idempotencyFailComplete, 1, "one transaction should be created")
	_, err = json.Marshal(idempotencyFailComplete)
	suite.Require().NoError(err)

	// bitflyer does not send us back what we sent it
	// so we end up in an odd space if we change amount or other inputs
	// which is ok, it just makes for odd checks
	// in this particular case, we return the original transactions with an "failed" status
	// which is why we do not need to modify the number amounts
	//
	// the invalid-input part is what will put the transaction in a different file
	// so that we do not send to eyeshade

	// two := decimal.NewFromFloat(1.9)
	// settlementTx2.Amount = two
	// settlementTx2.Probi = altcurrency.BAT.ToProbi(settlementTx2.Amount)
	// settlementTx2.BATPlatformFee = altcurrency.BAT.ToProbi(decimal.NewFromFloat(0.1))

	// idempotencyFailNote := idempotencyFailComplete[0].Note
	// suite.Require().Equal("OTHER_ERROR: Duplicate transfer_id and different parameters. transferID: "+idempotencyFailComplete[0].BitflyerTransferID(), idempotencyFailNote)
	// idempotencyFailCompleteExpected, err := json.Marshal([]custodian.Transaction{
	// 	transactionSubmitted("complete", settlementTx2, idempotencyFailNote),
	// })
	// suite.Require().NoError(err)
	// suite.Require().JSONEq(
	// 	string(idempotencyFailCompleteExpected),
	// 	string(idempotencyFailCompleteActual),
	// )
}

func (suite *BitflyerMockSuite) TestPrepareRequests() {
	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)
	price := decimal.NewFromFloat(0.25)

	ctx := context.Background()

	address1 := uuid.NewV4()
	address2 := uuid.NewV4()
	address3 := uuid.NewV4()

	settlementTx1 := settlementTransaction("1.9", address1.String())
	settlementTx2 := settlementTransaction("1.3", address1.String())
	settlementTx3 := settlementTransaction("1.1", address2.String())
	settlementTx4 := settlementTransaction("9999999999999", address3.String())

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	preparedTransactions, err := PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx1, settlementTx2, settlementTx3, settlementTx4},
		false,
		"tipping",
	)
	suite.Require().NoError(err)

	totalTxns := 0
	for _, batches := range preparedTransactions.AggregateTransactionBatches {
		totalTxns += len(batches)
	}

	suite.Require().Equal(3, totalTxns, "three agrregated transactions should be prepared")
	suite.Require().Len(preparedTransactions.NotSubmittedTransactions, 0, "zero transaction should be skipped")

	suite.client.EXPECT().
		FetchQuote(ctx, currencyCode, true).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         price,
		}, nil)

	preparedTransactions, err = PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx1, settlementTx2, settlementTx3, settlementTx4},
		true,
		"tipping",
	)
	suite.Require().NoError(err)

	totalTxns = 0
	for _, batches := range preparedTransactions.AggregateTransactionBatches {
		totalTxns += len(batches)
	}
	suite.Require().Equal(2, totalTxns, "two aggregated transaction should be prepared")
	suite.Require().Len(preparedTransactions.NotSubmittedTransactions, 1, "one transaction should be skipped")

}
