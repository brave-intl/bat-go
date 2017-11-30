package grant

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/pressly/lg"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/square/go-jose"
	"golang.org/x/crypto/ed25519"
)

const (
	lowerTxLimit = 20
	upperTxLimit = 120
)

var (
	SettlementDestination     = os.Getenv("BAT_SETTLEMENT_ADDRESS")
	GrantSignatorPublicKeyHex = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
	GrantWalletPublicKeyHex   = os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	GrantWalletPrivateKeyHex  = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	GrantWalletCardId         = os.Getenv("GRANT_WALLET_CARD_ID")
	grantPublicKey            ed25519.PublicKey
	grantWallet               *uphold.UpholdWallet
	refreshBalance            = true // for testing we can disable balance refresh
	testSubmit                = true // for testing we can disable testing tx submit
)

func InitGrantService() error {
	grantPublicKey, _ = hex.DecodeString(GrantSignatorPublicKeyHex)

	if os.Getenv("ENV") == "production" && refreshBalance != true {
		return errors.New("refreshBalance must be true in production!!")
	}
	if os.Getenv("ENV") == "production" && testSubmit != true {
		return errors.New("testSubmit must be true in production!!")
	}

	var info wallet.WalletInfo
	info.Provider = "uphold"
	info.ProviderId = GrantWalletCardId
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	var pubKey httpsignature.Ed25519PubKey
	var privKey ed25519.PrivateKey
	var err error

	pubKey, _ = hex.DecodeString(GrantWalletPublicKeyHex)
	privKey, _ = hex.DecodeString(GrantWalletPrivateKeyHex)

	grantWallet, err = uphold.New(info, privKey, pubKey)
	if err != nil {
		return err
	}

	return nil
}

type Grant struct {
	AltCurrency       *altcurrency.AltCurrency `json:"altcurrency"`
	GrantId           uuid.UUID                `json:"grantId"`
	Probi             decimal.Decimal          `json:"probi"`
	PromotionId       uuid.UUID                `json:"promotionId"`
	MaturityTimestamp int64                    `json:"maturityTime"` // FIXME
	ExpiryTimestamp   int64                    `json:"expiryTime"`   // FIXME
}

// ByProbi implements sort.Interface for []Grant based on the Probi field.
type ByProbi []Grant

func (a ByProbi) Len() int           { return len(a) }
func (a ByProbi) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByProbi) Less(i, j int) bool { return a[i].Probi.LessThan(a[j].Probi) }

func FromCompactJWS(s string) (*Grant, error) {
	jws, err := jose.ParseSigned(s)
	if err != nil {
		return nil, err
	}
	for _, sig := range jws.Signatures {
		if sig.Header.Algorithm != "EdDSA" {
			return nil, errors.New("Error unsupported JWS algorithm")
		}
	}
	jwk := jose.JSONWebKey{Key: grantPublicKey}
	grantBytes, err := jws.Verify(jwk)
	if err != nil {
		return nil, err
	}

	var grant Grant
	err = json.Unmarshal(grantBytes, &grant)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

type RedeemGrantsRequest struct {
	Grants      []string          `json:"grants" valid:"compactjws"`
	WalletInfo  wallet.WalletInfo `json:"wallet" valid:"required"`
	Transaction string            `json:"transaction" valid:"base64"`
}

// Verify one or more grants to fufill the included transaction for wallet
// Note that this is destructive, on success consumes grants.
// Further calls to Verify with the same request will fail.
//
// 1. Check grant signatures and decode
//
// 2. Check transaction signature and decode, enforce minimum transaction amount
//
// 3. Sort decoded grants, largest probi to smallest
//
// 4. Sum from largest to smallest until value is gt transaction amount
//
// 5. Fail if there are leftover grants
//
// 6. Iterate through grants and check that:
//
// a) this wallet has not yet redeemed a grant for the given promotionId
//
// b) this grant has not yet been redeemed by any wallet
//
// Returns transaction info for grant fufillment
func (req *RedeemGrantsRequest) Verify(ctx context.Context) (*wallet.TransactionInfo, error) {
	log := lg.Log(ctx)

	// 1. Check grant signatures and decode
	grants := make([]Grant, 0, len(req.Grants))
	for _, grantJWS := range req.Grants {
		grant, err := FromCompactJWS(grantJWS)
		if err != nil {
			return nil, err
		}
		grants = append(grants, *grant)
	}

	// 2. Check transaction signature and decode, enforce transaction checks
	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		return nil, err
	}
	// this ensures we have a valid wallet if refreshBalance == true
	balance, err := userWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}
	// NOTE for uphold provider we currently check against user provided publicKey
	//      thus this check does not protect us from a valid fake signature
	txInfo, err := userWallet.VerifyTransaction(req.Transaction)
	if err != nil {
		return nil, err
	}
	if *txInfo.AltCurrency != altcurrency.BAT {
		return nil, errors.New("Only grants submitted with BAT transactions are supported")
	}
	if txInfo.Probi.LessThan(decimal.Zero) {
		return nil, errors.New("Included transaction cannot be for negative BAT")
	}
	if txInfo.Probi.LessThan(altcurrency.BAT.ToProbi(decimal.New(lowerTxLimit, 0))) {
		return nil, errors.New(fmt.Sprintf("Included transaction must be for a minimum of %d BAT", lowerTxLimit))
	}
	if txInfo.Probi.LessThan(balance.SpendableProbi) {
		return nil, errors.New("Wallet has enough funds to cover transaction")
	}
	if txInfo.Probi.GreaterThan(altcurrency.BAT.ToProbi(decimal.New(upperTxLimit, 0))) {
		return nil, errors.New(fmt.Sprintf("Included transaction must be for a maxiumum of %d BAT", upperTxLimit))
	}
	if txInfo.Destination != SettlementDestination {
		return nil, errors.New("Included transactions must have settlement as their destination")
	}

	if testSubmit {
		// TODO remove this once we can retrieve publicKey info from uphold
		// NOTE We check the signature on the included transaction by attempting to submit it.
		//      We rely on the fact that uphold verifies signatures before doing balance checking.
		//      We are expecting a balance error, if we get a signature error we have
		//      the wrong publicKey.
		_, err = userWallet.SubmitTransaction(req.Transaction)
		if err == nil {
			return nil, errors.New("An included transaction unexpectedly succeeded")
		} else {
			if wallet.IsInvalidSignature(err) {
				return nil, errors.New("The included transaction was signed with the wrong publicKey!")
			} else if !wallet.IsInsufficientBalance(err) {
				return nil, errors.New("Error while test submitting the included transaction: " + err.Error())
			}
		}
	}

	// 3. Sort decoded grants, largest probi to smallest
	sort.Sort(sort.Reverse(ByProbi(grants)))

	// 4. Sum from largest to smallest until value is gt transaction amount
	needed := txInfo.Probi.Sub(balance.SpendableProbi)

	sumProbi := decimal.New(0, 1)
	for _, grant := range grants {
		if sumProbi.GreaterThanOrEqual(needed) {
			// 5. Fail if there are leftover grants
			return nil, errors.New("More grants included than are needed to fufill included transaction")
		}
		if *grant.AltCurrency != altcurrency.BAT {
			return nil, errors.New("All grants must be in BAT")
		}
		sumProbi = sumProbi.Add(grant.Probi)
	}

	// 6. Iterate through grants and check that:
	for _, grant := range grants {
		redeemedGrants, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionId.String()+":grants")
		if err != nil {
			return nil, err
		}
		defer redeemedGrants.Close()
		redeemedWallets, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionId.String()+":wallets")
		if err != nil {
			return nil, err
		}
		defer redeemedWallets.Close()

		result, err := redeemedGrants.Add(grant.GrantId.String())
		if err != nil {
			return nil, err
		}
		if result != true {
			// a) this wallet has not yet redeemed a grant for the given promotionId
			log.Error("Attempt to redeem previously redeemed grant!!!")
			return nil, errors.New(fmt.Sprintf("Grant %s has already been redeemed", grant.GrantId))
		}

		result, err = redeemedWallets.Add(req.WalletInfo.ProviderId)
		if err != nil {
			return nil, err
		}
		if result != true {
			// b) this grant has not yet been redeemed by any wallet
			log.Error("Attempt to redeem multiple grants from one promotion by the same wallet!!!")
			return nil, errors.New(fmt.Sprintf("Wallet %s has already redeemed a grant from this promotion", req.WalletInfo.ProviderId))
		}
	}

	var redeemTxInfo wallet.TransactionInfo
	{
		tmp := altcurrency.BAT
		redeemTxInfo.AltCurrency = &tmp
	}
	redeemTxInfo.Probi = sumProbi
	redeemTxInfo.Destination = req.WalletInfo.ProviderId
	return &redeemTxInfo, nil
}

func (req *RedeemGrantsRequest) Redeem(ctx context.Context) (*wallet.TransactionInfo, error) {
	grantFulfillmentInfo, err := req.Verify(ctx)
	if err != nil {
		return nil, err
	}

	// TODO consider early response here, best to avoid exposing any signature timing

	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		return nil, err
	}

	// fund user wallet with probi from grants
	_, err = grantWallet.Transfer(*grantFulfillmentInfo.AltCurrency, grantFulfillmentInfo.Probi, grantFulfillmentInfo.Destination)
	if err != nil {
		return nil, err
	}

	// send settlement transaction to wallet provider
	//
	// NOTE Verify (by way of VerifyTransaction) guards against transactions that seek to exploit parser differences
	// such as including additional fields that are not understood by this wallet provider implementation but may
	// be understood by the upstream wallet provider.
	settlementInfo, err := userWallet.SubmitTransaction(req.Transaction)
	if err != nil {
		return nil, err
	}

	return settlementInfo, nil
}
