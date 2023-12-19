//go:build integration && vpn
// +build integration,vpn

package zebpay

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strconv"
	"testing"

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ZebpayTestSuite struct {
	suite.Suite
	signingKey cryptography.PrivateKey
	apikey     string
}

func TestZebpayTestSuite(t *testing.T) {
	suite.Run(t, new(ZebpayTestSuite))
}

func (suite *ZebpayTestSuite) SetupTest() {
	apiKey := os.Getenv("ZEBPAY_API_KEY")
	signingKey := os.Getenv("ZEBPAY_SIGNING_KEY")
	if signingKey != "" && apiKey != "" {
		// parse the key from env variable
		block, _ := pem.Decode([]byte(pemString))
		key, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
		// set the private key to secret
		suite.signingKey = key
		suite.apiKey = apiKey
	}
}

func (suite *ZebpayTestSuite) TestBulkTransfer() {
	ctx := context.Background()
	client, err := New()
	suite.Require().NoError(err, "Must be able to correctly initialize the client")
	five := decimal.NewFromFloat(5)
	destination, err := strconv.ParseInt(os.Getenv("ZEBPAY_TEST_DESTINATION"), 10, 64)
	suite.Require().NoError(err, "Must be able to get the test destination")
	from := os.Getenv("ZEBPAY_TEST_FROM")
	id := uuid.New()
	opts := &ClientOpts{
		APIKey:     suite.apiKey,
		SigningKey: suite.signingKey,
	}

	resp, err := client.BulkTransfer(ctx, opts, NewBulkTransferRequest(&transfer{
		ID:          id,
		Destination: destination,
		Amount:      five,
		From:        from,
	}))
	suite.Require().NoError(err, "Must be able to perform the bulk transfer")
	// should have a success value in data
	suite.Require().True(resp.Data == "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED")

	// check on the transfer
	status, err := client.CheckTransfer(ctx, opts, id)
	suite.Require().NoError(err, "Must be able to perform the bulk transfer")

	// code should be pending or success, anything else is a failure
	suite.Require().True(status.Code == TransferPendingCode || status.Code == TransferSuccessCode)
}
