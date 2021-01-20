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
	return settlement.Transaction{
		AltCurrency:      &bat,
		Amount:           amountDecimal,
		Probi:            amountDecimal.Mul(decimal.New(1, 18)),
		BATPlatformFee:   amountDecimal.Div(decimal.NewFromFloat(19)).Mul(decimal.New(1, 18)),
		Destination:      address,
		SettlementID:     uuid.NewV4().String(),
		WalletProvider:   "bitflyer",
		WalletProviderID: uuid.NewV4().String(),
	}
}

func (suite *BitflyerSuite) TestFormData() {
	ctx := context.Background()

	settlementTx := settlementTransaction("1.9", "2492cdba-d33c-4a8d-ae5d-8799a81c61c2")
	tmpFile := suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx,
	})
	defer os.Remove(tmpFile.Name())
	payoutFiles, err := IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile.Name()},
	)
	suite.Require().NoError(err)
	completed := (*payoutFiles)["complete"]
	suite.Require().Len(completed, 1, "one transaction should be created")

	settlementTx.Amount = settlementTx.Amount.Add(decimal.NewFromFloat(1))
	tmpFile = suite.writeSettlementFiles(suite.token, []settlement.Transaction{
		settlementTx,
	})
	defer os.Remove(tmpFile.Name())
	payoutFiles, err = IterateRequest(
		ctx,
		"upload",
		suite.client,
		[]string{tmpFile.Name()},
	)
	suite.Require().NoError(err)
	completed = (*payoutFiles)["complete"]
	suite.Require().Len(completed, 1, "one transaction should be created")
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
