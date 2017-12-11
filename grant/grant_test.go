package grant

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/pressly/lg"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

func TestFromCompactJWS(t *testing.T) {
	GrantSignatorPublicKeyHex = "f2eb37b5eb30ad5b888c680ab8848a46fc2a6be81324de990ad20dc9b6e569fe"
	registerGrantInstrumentation = false
	InitGrantService()

	expectedGrantJSON := []byte(`{"altcurrency":"BAT","grantId":"9614ade7-58af-4df0-86c6-2f70051b43de","probi":"30000000000000000000","promotionId":"880309fc-df27-40a8-8d51-9cf39885e61d","maturityTime":1511769862,"expiryTime":1513843462}`)

	jwsGrant := "eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiI5NjE0YWRlNy01OGFmLTRkZjAtODZjNi0yZjcwMDUxYjQzZGUiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiI4ODAzMDlmYy1kZjI3LTQwYTgtOGQ1MS05Y2YzOTg4NWU2MWQiLCJtYXR1cml0eVRpbWUiOjE1MTE3Njk4NjIsImV4cGlyeVRpbWUiOjE1MTM4NDM0NjJ9.OOEBJUHPE21OyFw5Vq1tRTxYQc7aEL-KL5Lb4nb1TZn_3LFkXEPY7bNo0GhJ6k9X2UkZ19rnfBbpXHKuqBupDA"

	expectedGrant := Grant{}
	err := json.Unmarshal(expectedGrantJSON, &expectedGrant)
	if err != nil {
		t.Error("unexpected error")
	}

	grant, err := FromCompactJWS(jwsGrant)
	if err != nil {
		t.Error("unexpected error")
		t.Error(err)
	}

	if !reflect.DeepEqual(*grant, expectedGrant) {
		t.Error("grant not equal")
	}

	// TODO add incorrect signature tests
}

func TestVerifyAndConsume(t *testing.T) {
	GrantSignatorPublicKeyHex = "f2eb37b5eb30ad5b888c680ab8848a46fc2a6be81324de990ad20dc9b6e569fe"
	SettlementDestination = "foo@bar.com"
	refreshBalance = false
	testSubmit = false
	registerGrantInstrumentation = false
	InitGrantService()

	grants := []string{"eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiI5NjE0YWRlNy01OGFmLTRkZjAtODZjNi0yZjcwMDUxYjQzZGUiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiI4ODAzMDlmYy1kZjI3LTQwYTgtOGQ1MS05Y2YzOTg4NWU2MWQiLCJtYXR1cml0eVRpbWUiOjE1MTE3Njk4NjIsImV4cGlyeVRpbWUiOjE1MTM4NDM0NjJ9.OOEBJUHPE21OyFw5Vq1tRTxYQc7aEL-KL5Lb4nb1TZn_3LFkXEPY7bNo0GhJ6k9X2UkZ19rnfBbpXHKuqBupDA"}
	walletInfo := wallet.Info{}
	walletInfo.Provider = "uphold"
	{
		tmp := altcurrency.BAT
		walletInfo.AltCurrency = &tmp
	}

	walletInfo.ProviderID = uuid.NewV4().String()
	walletInfo.PublicKey = "424073b208e97af51cab7a389bcfe6942a3b7c7520fe9dab84f311f7846f5fcf"
	walletInfo.LastBalance = &wallet.Balance{}

	transaction := "eyJoZWFkZXJzIjp7ImRpZ2VzdCI6IlNIQS0yNTY9WFg0YzgvM0J4ejJkZWNkakhpY0xWaXJ5dTgxbWdGNkNZTTNONFRHc0xoTT0iLCJzaWduYXR1cmUiOiJrZXlJZD1cInByaW1hcnlcIixhbGdvcml0aG09XCJlZDI1NTE5XCIsaGVhZGVycz1cImRpZ2VzdFwiLHNpZ25hdHVyZT1cIjI4TitabzNodlRRWmR2K2trbGFwUE5IY29OMEpLdWRiSU5GVnlOSm0rWDBzdDhzbXdzYVlHaTJQVHFRbjJIVWdacUp4Q2NycEpTMWpxZHdyK21RNEN3PT1cIiJ9LCJvY3RldHMiOiJ7XCJkZW5vbWluYXRpb25cIjp7XCJhbW91bnRcIjpcIjI1XCIsXCJjdXJyZW5jeVwiOlwiQkFUXCJ9LFwiZGVzdGluYXRpb25cIjpcImZvb0BiYXIuY29tXCJ9In0="

	logger := logrus.New()
	ctx := context.Background()
	ctx = lg.WithLoggerContext(ctx, logger)

	request := RedeemGrantsRequest{grants, walletInfo, transaction}
	_, err := request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("Should not be able to redeem without a valid claim on a grant")
	}

	claimReq := ClaimGrantRequest{walletInfo}
	claimReq.Claim(ctx, "9614ade7-58af-4df0-86c6-2f70051b43de")

	_, err = request.VerifyAndConsume(ctx)
	if err != nil {
		t.Error(err)
	}

	_, err = request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("expected re-redeem with the same grant to fail")
	}

	// another grant from the same promotionId
	grants = []string{"eyJhbGciOiJFZERTQSIsImtpZCI6IiJ9.eyJhbHRjdXJyZW5jeSI6IkJBVCIsImdyYW50SWQiOiJmODdlN2ZiNC0wZjgwLTQwYWQtYjA5Mi04NGY3MGU0NDg0MjEiLCJwcm9iaSI6IjMwMDAwMDAwMDAwMDAwMDAwMDAwIiwicHJvbW90aW9uSWQiOiI4ODAzMDlmYy1kZjI3LTQwYTgtOGQ1MS05Y2YzOTg4NWU2MWQiLCJtYXR1cml0eVRpbWUiOjE1MTE3Njk4NjIsImV4cGlyeVRpbWUiOjE1MTM4NDM0NjJ9.AZ13bwEbmIt-ji2gWqg4ofRd1dQ8D1h0BnypWkg_AsKlfU5ne3LcSnfsMkamwv_0Vz4OgApwDl5feC2lMRHFDg"}

	// claim this grant as well to ensure we are testing re-redeem with same wallet and promotion
	claimReq.Claim(ctx, "f87e7fb4-0f80-40ad-b092-84f70e448421")

	request = RedeemGrantsRequest{grants, walletInfo, transaction}
	_, err = request.VerifyAndConsume(ctx)
	if err == nil {
		t.Error("expected re-redeem with the same wallet and promotion to fail")
	}

	// TODO add more tests
}
