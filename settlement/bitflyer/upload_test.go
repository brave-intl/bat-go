package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

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

func transactionSubmitted(status string, tx settlement.Transaction) settlement.Transaction {
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
	}
}

func (suite *BitflyerSuite) TestFormData() {
	ctx := context.Background()
	address := "2492cdba-d33c-4a8d-ae5d-8799a81c61c2"
	settlementTx1 := settlementTransaction("2", address)
	tmpFile1 := suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx1,
	})
	defer os.Remove(tmpFile1.Name())
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
		transactionSubmitted("complete", settlementTx1),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(completeSerialized), string(mCompleted))

	payoutFiles, err = IterateRequest(
		ctx,
		"checkstatus",
		suite.client,
		[]string{tmpFile1.Name()},
	)
	suite.Require().NoError(err)
	completedStatus := (*payoutFiles)["complete"]
	suite.Require().Len(completedStatus, 1, "one transaction should be created")
	completeSerializedStatus, err := json.Marshal(completedStatus)
	suite.Require().NoError(err)

	mCompletedStatus, err := json.Marshal([]settlement.Transaction{
		transactionSubmitted("complete", settlementTx1),
	})
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(completeSerializedStatus), string(mCompletedStatus))

	//  100000000000000000
	// 1900000000000000000

	// make a new tx that will conflict with previous
	settlementTx2 := settlementTransaction("3", address)
	settlementTx2.SettlementID = settlementTx1.SettlementID
	tmpFile2 := suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx2,
	})
	defer os.Remove(tmpFile2.Name())
	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile2.Name()},
	)
	completed = (*payoutFiles)["complete"]
	completeSerialized, err = json.Marshal(completed)
	suite.Require().NoError(err)

	settlementTx2.ProviderID = bitflyer.GenerateTransferID(&settlementTx2) // add bitflyer transaction hash
	mCompleted, err = json.Marshal([]settlement.Transaction{               // serialize for comparison (decimal.Decimal does not do so well)
		transactionSubmitted("complete", settlementTx2),
	})
	// this test must fail otherwise it is not idempotent
	// suite.Require().Error(err)
	// suite.Require().JSONEq(string(completeSerialized), string(mCompleted))
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
