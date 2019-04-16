package grant

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	raven "github.com/getsentry/raven-go"
	"github.com/pressly/lg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
)

// RedeemGrantsRequest a request to redeem the included grants for the wallet whose information
// is included in order to fulfill the included transaction
type RedeemGrantsRequest struct {
	Grants      []string    `json:"grants" valid:"compactjws"`
	WalletInfo  wallet.Info `json:"wallet" valid:"required"`
	Transaction string      `json:"transaction" valid:"base64"`
}

// RedemptionDisabled due to fail safe condition
func RedemptionDisabled() bool {
	return safeMode || breakerTripped
}

// VerifyAndConsume one or more grants to fulfill the included transaction for wallet
// Note that this is destructive, on success consumes grants.
// Further calls to Verify with the same request will fail as the grants are consumed.
//
// 1. Check grant signatures and decode
//
// 2. Check transaction signature and decode, enforce minimum transaction amount
//
// 3. Sort decoded grants, closest expiration to furthest
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
func (service *Service) VerifyAndConsume(ctx context.Context, req *RedeemGrantsRequest) (*wallet.TransactionInfo, error) {
	log := lg.Log(ctx)
	// 1. Check grant signatures and decode
	grants, err := DecodeGrants(grantPublicKey, req.Grants)
	if err != nil {
		return nil, err
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
		return nil, errors.New("only grants submitted with BAT transactions are supported")
	}
	if txInfo.Probi.LessThan(decimal.Zero) {
		return nil, errors.New("included transaction cannot be for negative BAT")
	}
	if txInfo.Probi.LessThan(altcurrency.BAT.ToProbi(decimal.New(lowerTxLimit, 0))) {
		return nil, fmt.Errorf("included transaction must be for a minimum of %d BAT", lowerTxLimit)
	}
	if txInfo.Probi.GreaterThan(altcurrency.BAT.ToProbi(decimal.New(upperTxLimit, 0))) {
		return nil, fmt.Errorf("included transaction must be for a maxiumum of %d BAT", upperTxLimit)
	}
	if txInfo.Destination != SettlementDestination {
		return nil, errors.New("included transactions must have settlement as their destination")
	}

	// 3. Sort decoded grants, closest expiration to furthest
	sort.Sort(ByExpiryTimestamp(grants))

	// 4. Sum until value is gt transaction amount
	sumProbi := decimal.New(0, 1)
	for _, grant := range grants {
		if sumProbi.GreaterThanOrEqual(txInfo.Probi) {
			// 5. Fail if there are leftover grants
			return nil, errors.New("More grants included than are needed to fulfill included transaction")
		}
		if *grant.AltCurrency != altcurrency.BAT {
			return nil, errors.New("All grants must be in BAT")
		}
		sumProbi = sumProbi.Add(grant.Probi)
	}

	if txInfo.Probi.GreaterThan(balance.SpendableProbi.Add(sumProbi)) {
		return nil, errors.New("wallet does not have enough funds to cover transaction")
	}

	// should be reasonable since we limit the redeem endpoint to a maximum of 1 simultaneous in-flight request
	ugpBalance, err := grantWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}

	if sumProbi.GreaterThan(ugpBalance.SpendableProbi) {
		safeMode = true
		raven.CaptureMessage("Hot wallet out of funds!!!", map[string]string{"out-of-funds": "true"})
		return nil, errors.New("ugp wallet lacks enough funds to fulfill grants")
	}

	var submitID string
	if testSubmit {
		var submitInfo *wallet.TransactionInfo
		// TODO remove this once we can retrieve publicKey info from uphold
		// NOTE We check the signature on the included transaction by submitting it but not confirming it
		submitInfo, err = userWallet.SubmitTransaction(req.Transaction, false)
		if err != nil {
			if wallet.IsInvalidSignature(err) {
				return nil, errors.New("the included transaction was signed with the wrong publicKey")
			} else if !wallet.IsInsufficientBalance(err) {
				return nil, errors.New("error while test submitting the included transaction: " + err.Error())
			}
		}
		submitID = submitInfo.ID
	}

	// 6. Iterate through grants and check that:
	for _, grant := range grants {
		claimedID, err := service.datastore.GetClaimantProviderID(grant)
		if err == nil {
			// if claimed it was by this wallet
			if req.WalletInfo.ProviderID != claimedID {
				log.Error("Attempt to redeem previously claimed by another wallet!!!")
				return nil, errors.New("Grant claim does not match provided wallet")
			}
		}

		// the grant is mature
		if time.Now().Unix() < grant.MaturityTimestamp {
			return nil, errors.New("Grant is not yet redeemable as it is immature")
		}

		// the grant is not expired
		if time.Now().Unix() > grant.ExpiryTimestamp {
			return nil, errors.New("Grant is expired")
		}

		err = service.datastore.RedeemGrantForWallet(grant, req.WalletInfo)
		if err != nil {
			return nil, err
		}

		redeemedGrantsCounter.With(prometheus.Labels{"promotionId": grant.PromotionID.String()}).Inc()
	}

	var redeemTxInfo wallet.TransactionInfo
	{
		tmp := altcurrency.BAT
		redeemTxInfo.AltCurrency = &tmp
	}
	redeemTxInfo.Probi = sumProbi
	redeemTxInfo.Destination = req.WalletInfo.ProviderID
	redeemTxInfo.ID = submitID
	return &redeemTxInfo, nil
}

// GetRedeemedIDs returns a list of any grants that have already been redeemed
func (service *Service) GetRedeemedIDs(ctx context.Context, Grants []string) ([]string, error) {

	// 1. Check grant signatures and decode
	grants, err := DecodeGrants(grantPublicKey, Grants)
	if err != nil {
		return nil, err
	}
	grantCount := len(grants)
	results := make([]string, 0, grantCount)

	for _, grant := range grants {
		grantRedeemed, err := service.datastore.HasGrantBeenRedeemed(grant)
		if err != nil {
			return nil, err
		}
		if grantRedeemed {
			results = append(results, grant.GrantID.String())
		}
	}

	return results, nil
}

// Redeem the grants in the included response
func (service *Service) Redeem(ctx context.Context, req *RedeemGrantsRequest) (*wallet.TransactionInfo, error) {
	log := lg.Log(ctx)

	if RedemptionDisabled() {
		return nil, errors.New("Grant redemption has been disabled due to fail-safe condition")
	}

	grantFulfillmentInfo, err := service.VerifyAndConsume(ctx, req)
	if err != nil {
		return nil, err
	}

	submitID := grantFulfillmentInfo.ID

	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		conn := service.redisPool.Get()
		defer closers.Panic(conn)
		b := GetBreaker(&conn)

		incErr := b.Increment()
		if incErr != nil {
			log.Errorf("Could not increment the breaker!!!")
			raven.CaptureMessage("Could not increment the breaker!!!", map[string]string{"breaker": "true"})
			safeMode = true
		}

		log.Errorf("Could not get wallet %s from info after successful VerifyAndConsume", req.WalletInfo.ProviderID)
		raven.CaptureMessage("Could not get wallet after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
		return nil, err
	}

	// fund user wallet with probi from grants
	_, err = grantWallet.Transfer(*grantFulfillmentInfo.AltCurrency, grantFulfillmentInfo.Probi, grantFulfillmentInfo.Destination)
	if err != nil {
		conn := service.redisPool.Get()
		defer closers.Panic(conn)
		b := GetBreaker(&conn)

		incErr := b.Increment()
		if incErr != nil {
			log.Errorf("Could not increment the breaker!!!")
			raven.CaptureMessage("Could not increment the breaker!!!", map[string]string{"breaker": "true"})
			safeMode = true
		}

		log.Errorf("Could not fund wallet %s after successful VerifyAndConsume", req.WalletInfo.ProviderID)
		raven.CaptureMessage("Could not fund wallet after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
		return nil, err
	}

	// confirm settlement transaction previously sent to wallet provider
	var settlementInfo *wallet.TransactionInfo
	for tries := 5; tries >= 0; tries-- {
		if tries == 0 {
			conn := service.redisPool.Get()
			defer closers.Panic(conn)
			b := GetBreaker(&conn)

			incErr := b.Increment()
			if incErr != nil {
				log.Errorf("Could not increment the breaker!!!")
				raven.CaptureMessage("Could not increment the breaker!!!", map[string]string{"breaker": "true"})
				safeMode = true
			}

			log.Errorf("Could not submit settlement txn for wallet %s after successful VerifyAndConsume", req.WalletInfo.ProviderID)
			raven.CaptureMessage("Could not submit settlement txn after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
			return nil, err
		}
		// NOTE VerifyAndConsume (by way of VerifyTransaction) guards against transactions that seek to exploit parser differences
		// such as including additional fields that are not understood by this wallet provider implementation but may
		// be understood by the upstream wallet provider.
		settlementInfo, err = userWallet.ConfirmTransaction(submitID)
		if err == nil {
			break
		}
	}

	return settlementInfo, nil
}
