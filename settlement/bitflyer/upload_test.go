package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
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
}

func (suite *BitflyerSuite) SetupTest() {
	// mockCtrl := gomock.NewController(suite.T())
	// defer mockCtrl.Finish()
	// suite.client = mockbitflyer.NewMockClient(mockCtrl)
	client, err := bitflyer.New()
	suite.client = client
	suite.Require().NoError(err)
	suite.token = os.Getenv("BITFLYER_CLIENT_TOKEN")
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

func (suite *BitflyerSuite) TestFormData() {
	ctx := context.Background()
	address := "2492cdba-d33c-4a8d-ae5d-8799a81c61c2"
	settlementTx1 := settlementTransaction("2", address)
	tmpFile1 := suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx1,
	})
	defer func() { _ = os.Remove(tmpFile1.Name()) }()
	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile1.Name()},
	)
	suite.Require().NoError(err)
	completed := (*payoutFiles)["complete"]
	suite.Require().Len(completed, 1, "one transaction should be created")
	completeSerialized, err := json.Marshal(completed)
	suite.Require().NoError(err)

	settlementTx1.ProviderID = bitflyer.GenerateTransferID(&settlementTx1) // add bitflyer transaction hash
	mCompleted, err := json.Marshal([]settlement.Transaction{              // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx1, "SUCCESS"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(completeSerialized), string(mCompleted))

	var completedStatus []settlement.Transaction
	for {
		<-time.After(time.Second)
		payoutFiles, err = IterateRequest(
			ctx,
			"checkstatus",
			suite.client,
			[]string{tmpFile1.Name()},
		)
		suite.Require().NoError(err)
		completedStatus = (*payoutFiles)["complete"]
		fmt.Println("status", completedStatus[0].Note)
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

	// payoutFiles, err = IterateRequest(
	// 	ctx,
	// 	"upload",
	// 	suite.client,
	// 	[]string{tmpFile1.Name()},
	// )
	// suite.Require().NoError(err)
	// idempotencyFailComplete := (*payoutFiles)["complete"]
	// idempotencyFailInvalidInput := (*payoutFiles)["invalid-input"]
	// suite.Require().Len(idempotencyFailComplete, 0, "one transaction should be created")
	// idempotencyFailInvalidInputSerialized, err := json.Marshal(idempotencyFailInvalidInput)
	// suite.Require().NoError(err)

	// idempotencyFailInvalidInputMarshalled, err := json.Marshal([]settlement.Transaction{
	// 	transactionSubmitted("invalid-input", settlementTx1, "SUCCESS"),
	// })
	// suite.Require().NoError(err)
	// suite.Require().JSONEq(string(idempotencyFailInvalidInputMarshalled), string(idempotencyFailInvalidInputSerialized))

	//  100000000000000000
	// 1900000000000000000

	// make a new tx that will conflict with previous
	settlementTx2 := settlementTransaction("3", address)
	// fmt.Println("settlementTx1.Hash", settlementTx1.ProviderID)
	// fmt.Println("settlementTx1.Amount", settlementTx1.Amount.String())
	settlementTx2.SettlementID = settlementTx1.SettlementID
	settlementTx2.Destination = settlementTx1.Destination
	settlementTx2.WalletProviderID = settlementTx1.WalletProviderID
	settlementTx2.ProviderID = bitflyer.GenerateTransferID(&settlementTx2) // add bitflyer transaction hash
	// fmt.Println("settlementTx2.Hash", settlementTx2.ProviderID)
	tmpFile2 := suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx2,
	})
	defer func() { _ = os.Remove(tmpFile2.Name()) }()
	// fmt.Println("settlementTx1.Hash", settlementTx1.ProviderID)
	// fmt.Println("settlementTx1.Amount", settlementTx1.Amount.String())
	// fmt.Println("settlementTx2.Hash", settlementTx2.ProviderID)
	// fmt.Println("settlementTx2.Amount", settlementTx2.Amount.String())
	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile2.Name()},
	)
	suite.Require().NoError(err)
	idempotencyFailComplete := (*payoutFiles)["complete"]
	idempotencyFailInvalidInput := (*payoutFiles)["invalid-input"]
	// fmt.Println("settlementTx1.Hash", settlementTx1.ProviderID)
	// fmt.Println("settlementTx1.Amount", settlementTx1.Amount.String())
	// fmt.Println("settlementTx2.Hash", settlementTx2.ProviderID)
	// fmt.Println("settlementTx2.Amount", settlementTx2.Amount.String())
	// fmt.Println("idempotencyFailInvalidInput[0].Amount", idempotencyFailInvalidInput[0].Amount.String())

	suite.Require().Len(idempotencyFailComplete, 0, "one transaction should be created")
	idempotencyFailInvalidInputActual, err := json.Marshal(idempotencyFailInvalidInput)
	suite.Require().NoError(err)

	// bitflyer does not send us back what we sent it
	// so we end up in an odd space if we change amount or other inputs
	// which is ok, it just makes for odd checks
	// in this particular case, we return the original transactions with an "invalid-input" status
	// which is why we do not need to modify the number amounts
	//
	// the invalid-input part is what will put it in a different file
	// so that we do not send to eyeshade
	idempotencyFailInvalidInputExpected, err := json.Marshal([]settlement.Transaction{
		transactionSubmitted("invalid-input", settlementTx2, "SUCCESS"),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(
		string(idempotencyFailInvalidInputExpected),
		string(idempotencyFailInvalidInputActual),
	)
}

func (suite *BitflyerSuite) writeSettlementFiles(tkn string, txs []settlement.Transaction) (filepath *os.File) {
	tmpDir := os.TempDir()
	tmpFile, err := ioutil.TempFile(tmpDir, "bat-go-test-bitflyer-upload-")
	suite.Require().NoError(err)

	groupedTxs := make(map[string][]settlement.Transaction)
	for _, tx := range txs {
		id := bitflyer.GenerateTransferID(&tx)
		groupedTxs[id] = append(groupedTxs[id], tx)
	}
	json, err := json.Marshal(groupedTxs)
	suite.Require().NoError(err)

	token := tkn
	if tkn == "" {
		token = "notoken"
	}
	fileContents := fmt.Sprintf(`{"api_key":"%s","transactions":%s}`, token, string(json))
	_, err = tmpFile.Write([]byte(fileContents))
	suite.Require().NoError(err)
	err = tmpFile.Close()
	suite.Require().NoError(err)
	return tmpFile
}
