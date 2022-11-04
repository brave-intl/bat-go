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

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/custodian"
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
	if os.Getenv("BITFLYER_LIVE") != "true" {
		suite.T().Skip("bitflyer side unable to settle")
	}
	client, err := bitflyer.New()
	suite.client = client
	suite.Require().NoError(err)
	token := os.Getenv("BITFLYER_TOKEN")
	if token == "" {
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
	} else {
		suite.token = token
	}
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

func settlementTransaction(amount, address string) custodian.Transaction {
	amountDecimal, _ := decimal.NewFromString(amount)
	bat := altcurrency.BAT
	fees := amountDecimal.Div(decimal.NewFromFloat(19))
	settlementID := uuid.NewV4().String()
	tx := custodian.Transaction{
		AltCurrency:      &bat,
		Currency:         "BAT",
		Amount:           amountDecimal,
		Probi:            amountDecimal.Mul(decimal.New(1, 18)),
		BATPlatformFee:   fees.Mul(decimal.New(1, 18)).Truncate(18),
		Destination:      address,
		Type:             "contribution",
		SettlementID:     settlementID,
		WalletProvider:   "bitflyer",
		WalletProviderID: address,
		TransferFee:      decimal.NewFromFloat(0),
		ExchangeFee:      decimal.NewFromFloat(0),
		Channel:          "brave.com",
	}
	tx.ProviderID = tx.TransferID()
	return tx
}

func transactionSubmitted(status string, tx custodian.Transaction, note string) custodian.Transaction {
	return custodian.Transaction{
		Status:           status,
		Channel:          tx.Channel,
		AltCurrency:      tx.AltCurrency,
		Currency:         tx.Currency,
		Type:             tx.Type,
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
	settlementTx0 := settlementTransaction("1.9", uuid.NewV4().String())

	preparedTransactions, err := PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx0},
		false,
		"tipping",
	)

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
	settlementTx0.ProviderID = settlementTx0.BitflyerTransferID()
	failedTxNote := failedTxs[0].Note
	fmt.Printf("%#v\n", failedTxNote)
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

	suite.client.SetAuthToken("")
	_, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil, // dry run first
	)
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

func (suite *BitflyerSuite) TestFormData() {
	// TODO: after we figure out why we are being blocked by bf enable
	ctx := context.Background()
	address := "ff3a0ead-c945-4c52-bcf7-9309319573de"
	sourceFrom := "tipping"
	duration, err := time.ParseDuration("4s")
	suite.Require().NoError(err)
	dryRunOptions := &bitflyer.DryRunOption{
		ProcessTimeSec: uint(duration.Seconds()),
	}

	settlementTx1 := settlementTransaction("1.9", address)

	preparedTransactions, err := PrepareRequests(
		ctx,
		suite.client,
		[]custodian.Transaction{settlementTx1},
		false,
		sourceFrom,
	)
	/*
		resultIteration := make(map[string]int)

		var payoutFiles *map[string][]custodian.Transaction
		for i := 0; i < 2; i++ {
			<-time.After(2 * time.Second)
			results, err := IterateRequest(
				ctx,
				"upload",
				suite.client,
				[]string{tmpFile1.Name()},
				sourceFrom,
				false,
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
		*preparedTransactions,
		dryRunOptions, // dry run first
	)
	suite.Require().NoError(err)
	completedDryRunTxs := payoutFiles["complete"]
	suite.Require().Len(completedDryRunTxs, 1, "one transaction should be created")

	completedDryRunBytes, err := json.Marshal(completedDryRunTxs)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = settlementTx1.BitflyerTransferID()
	expectedBytes, err := json.Marshal([]custodian.Transaction{ // serialize for comparison (decimal.Decimal does not do so well)
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

	settlementTx1.ProviderID = settlementTx1.BitflyerTransferID() // add bitflyer transaction hash
	mCompleted, err := json.Marshal([]custodian.Transaction{      // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(completeSerialized),
		string(mCompleted),
	)

	var completedStatus []custodian.Transaction
	for {
		<-time.After(time.Second)
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
		// fmt.Printf("checkstatus %#v\n", *payoutFiles)
		if len(completedStatus) > 0 {
			break
		}
	}
	suite.Require().Len(completedStatus, 1, "one transaction should be created")
	completeSerializedStatus, err := json.Marshal(completedStatus)
	suite.Require().NoError(err)

	mCompletedStatus, err := json.Marshal([]custodian.Transaction{
		transactionSubmitted("complete", settlementTx1, "EXECUTED"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(completeSerializedStatus), string(mCompletedStatus))

	// make a new tx that will conflict with previous
	settlementTx2 := settlementTransaction("2.85", address)
	settlementTx2.SettlementID = settlementTx1.SettlementID
	settlementTx2.Destination = settlementTx1.Destination
	settlementTx2.WalletProviderID = settlementTx1.WalletProviderID
	settlementTx2.ProviderID = settlementTx2.BitflyerTransferID() // add bitflyer transaction hash

	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		*preparedTransactions,
		nil,
	)
	suite.Require().NoError(err)
	idempotencyFailComplete := payoutFiles["complete"]
	suite.Require().Len(idempotencyFailComplete, 1, "one transaction should be created")
	idempotencyFailCompleteActual, err := json.Marshal(idempotencyFailComplete)
	suite.Require().NoError(err)

	// bitflyer does not send us back what we sent it
	// so we end up in an odd space if we change amount or other inputs
	// which is ok, it just makes for odd checks
	// in this particular case, we return the original transactions with an "failed" status
	// which is why we do not need to modify the number amounts
	//
	// the invalid-input part is what will put the transaction in a different file
	// so that we do not send to eyeshade
	idempotencyFailNote := idempotencyFailComplete[0].Note
	suite.Require().Equal("OTHER_ERROR: Duplicate transfer_id and different parameters", idempotencyFailNote)
	idempotencyFailCompleteExpected, err := json.Marshal([]custodian.Transaction{
		transactionSubmitted("failed", settlementTx2, idempotencyFailNote),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(idempotencyFailCompleteExpected),
		string(idempotencyFailCompleteActual),
	)
}
