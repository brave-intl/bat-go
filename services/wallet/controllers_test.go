// +build integration

package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *ControllersTestSuite) SetupTest() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *ControllersTestSuite) SettlementWallet() *uphold.Wallet {
	var pubKey httpsignature.Ed25519PubKey
	var privKey ed25519.PrivateKey
	var err error
	grantWalletPublicKeyHex := os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex := os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	providerID := os.Getenv("BAT_SETTLEMENT_ADDRESS")

	pubKey, err = hex.DecodeString(grantWalletPublicKeyHex)
	suite.Require().NoError(err, "grantWalletPublicKeyHex is invalid")
	privKey, err = hex.DecodeString(grantWalletPrivateKeyHex)
	suite.Require().NoError(err, "grantWalletPrivateKeyHex is invalid")

	bat := altcurrency.BAT
	return &uphold.Wallet{
		Info: wallet.Info{
			ID:          uuid.NewV4().String(),
			Provider:    "uphold",
			ProviderID:  providerID,
			AltCurrency: &bat,
			PublicKey:   hex.EncodeToString(pubKey),
		},
		PrivKey: privKey,
		PubKey:  pubKey,
	}
}

func (suite *ControllersTestSuite) TransferFunds(probi decimal.Decimal, destination string) *wallet.TransactionInfo {
	settlementWallet := suite.SettlementWallet()
	fmt.Printf("transferring: %d to %s\n", probi, destination)
	txn, err := settlementWallet.Transfer(altcurrency.BAT, probi, destination)
	suite.Require().NoError(err, "could not prepare wallet claim transaction")
	return txn
}

func (suite *ControllersTestSuite) TestClaimWallet() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres connection")

	service := &Service{
		datastore: pg,
	}

	anonCard1 := suite.CreateAndFundUserWallet(service)
	anonCard2 := suite.CreateAndFundUserWallet(service)
	anonCard3 := suite.CreateAndFundUserWallet(service)
	anonCard4 := suite.CreateAndFundUserWallet(service)

	anonCard1AnonAddress := suite.CreateAnonymousCards(anonCard1, "anonymous")
	anonCard2AnonAddress := suite.CreateAnonymousCards(anonCard2, "anonymous")
	anonAddressCard1UUID := uuid.Must(uuid.FromString(anonCard1AnonAddress))
	anonAddressCard2UUID := uuid.Must(uuid.FromString(anonCard2AnonAddress))

	settlementWallet := suite.SettlementWallet()
	zero := decimal.NewFromFloat(0.0)

	// tie first one to settlement wallet id to tie them all back
	_ = suite.ClaimCard(service, anonCard1, settlementWallet.Info.ProviderID, http.StatusOK, anonCard1.Info.LastBalance.TotalProbi, nil)
	// zero amount does not error
	_ = suite.ClaimCard(service, anonCard2, anonCard1.Info.ProviderID, http.StatusOK, zero, &anonAddressCard1UUID)
	_ = suite.ClaimCard(service, anonCard2, anonCard1.Info.ProviderID, http.StatusOK, anonCard2.Info.LastBalance.TotalProbi, nil)
	_ = suite.ClaimCard(service, anonCard3, anonCard2.Info.ProviderID, http.StatusOK, anonCard3.Info.LastBalance.TotalProbi, nil)
	// up to 3 cards can be claimed
	_ = suite.ClaimCard(service, anonCard4, anonCard3.Info.ProviderID, http.StatusConflict, anonCard4.Info.LastBalance.TotalProbi, nil)
	// claiming again results in ok
	_ = suite.ClaimCard(service, anonCard3, anonCard2.Info.ProviderID, http.StatusOK, zero, &anonAddressCard2UUID)
}

func (suite *ControllersTestSuite) ClaimCard(
	service *Service,
	wallet *uphold.Wallet,
	providerID string,
	status int,
	probi decimal.Decimal,
	anonymousAddress *uuid.UUID,
) *httptest.ResponseRecorder {
	handler := PostClaimWalletCompat(service)

	txn, err := wallet.PrepareTransaction(altcurrency.BAT, probi, providerID, "bat-go:wallet-claim:TestClaimCard")
	suite.Require().NoError(err, "could not prepare wallet claim transaction")

	claimRequest := struct {
		SignedTx         string     `json:"signedTx" valid:"base64"`
		AnonymousAddress *uuid.UUID `json:"anonymousAddress,omitempty"`
	}{
		SignedTx:         txn,
		AnonymousAddress: anonymousAddress,
	}

	body, err := json.Marshal(&claimRequest)
	suite.Require().NoError(err, "could not marshal claim request")

	req, err := http.NewRequest("POST", "/v1/wallet/{paymentID}/claim", bytes.NewBuffer(body))
	suite.Require().NoError(err, "wallet claim request could not be created")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("paymentID", wallet.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func (suite *ControllersTestSuite) CreateAnonymousCards(upholdWallet *uphold.Wallet, network string) string {
	depositAddr, err := upholdWallet.CreateCardAddress(network)
	suite.Require().NoError(err, "Failed to create card on network")
	return depositAddr
}

func (suite *ControllersTestSuite) CreateAndFundUserWallet(service *Service) *uphold.Wallet {
	walletID := uuid.NewV4()
	name := `test-uphold-anon-claim-` + walletID.String()
	client, err := vaultsigner.Connect()
	suite.Require().NoError(err, "Failed to connect to vaultsigner")

	signer, err := vaultsigner.New(client, name)
	suite.Require().NoError(err, "Failed create new vaultsigner signer")
	bat := altcurrency.BAT
	walletInfo := wallet.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   signer.String(),
	}

	upholdWallet := &uphold.Wallet{
		Info:    walletInfo,
		PrivKey: signer,
		PubKey:  signer,
	}

	reg, err := upholdWallet.PrepareRegistration(name)
	suite.Require().NoError(err, "Failed to prepare registration for uphold")

	var publicKey httpsignature.Ed25519PubKey
	publicKey, err = hex.DecodeString(walletInfo.PublicKey)
	suite.Require().NoError(err, "Failed to generate public key")

	upholdWallet = &uphold.Wallet{
		Info:    walletInfo,
		PrivKey: ed25519.PrivateKey{},
		PubKey:  publicKey,
	}

	err = upholdWallet.SubmitRegistration(reg)
	suite.Require().NoError(err, "Failed to complete uphold registration")

	depositAddr := suite.CreateAnonymousCards(upholdWallet, "ethereum")
	walletInfo.ProviderID = depositAddr

	err = service.datastore.InsertWallet(&walletInfo)
	suite.Assert().NoError(err, "Save wallet should succeed")

	probi := decimal.NewFromFloat(1.0)
	_ = suite.TransferFunds(probi, walletInfo.ProviderID)
	_, err = upholdWallet.GetBalance(true)
	suite.Assert().NoError(err, "balance should be refreshed")
	return upholdWallet
}
