package grant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// RedeemGrantsRequest a request to redeem the included grants for the wallet whose information
// is included in order to fulfill the included transaction
type RedeemGrantsRequest struct {
	WalletInfo  wallet.Info `json:"wallet" valid:"required"`
	Transaction string      `json:"transaction" valid:"base64"`
}

// RedemptionDisabled due to fail safe condition
func RedemptionDisabled() bool {
	return safeMode
}

// Consume one or more grants to fulfill the included transaction for wallet
// Note that this is destructive, on success consumes grants.
// Further calls to Verify with the same request will fail as the grants are consumed.
//
// 1. Sort grants, closest expiration to furthest, short circuit if no grants
//
// 2. Enforce transaction checks and verify transaction signature
//
// 3. Sum from largest to smallest until value is gt transaction amount
//
// 4. Iterate through grants and check that:
//
// a) this wallet has not yet redeemed a grant for the given promotionId
//
// b) this grant has not yet been redeemed by any wallet
//
// Returns transaction info for grant fufillment
func (service *Service) Consume(ctx context.Context, walletInfo wallet.Info, transaction string) (*wallet.TransactionInfo, error) {
	var txProbi *decimal.Decimal
	var redeemTxInfo wallet.TransactionInfo
	{
		tmp := altcurrency.BAT
		redeemTxInfo.AltCurrency = &tmp
	}

	promotionType := ""
	if len(transaction) == 0 { // We are draining ad grants
		promotionType = "{ads}"
	}

	// 1. Sort grants, closest expiration to furthest, short circuit if no grants
	unredeemedGrants, err := service.datastore.GetGrantsOrderedByExpiry(walletInfo, promotionType)
	if err != nil {
		return nil, errorutils.Wrap(err, "could not fetch grants ordered by expiration date")
	}

	if len(unredeemedGrants) == 0 {
		return nil, nil
	}

	// 2. Enforce transaction checks and verify transaction signature
	providerWallet, err := provider.GetWallet(walletInfo)
	if err != nil {
		return nil, err
	}
	userWallet, ok := providerWallet.(*uphold.Wallet)
	if !ok {
		return nil, errors.New("only uphold wallets are supported")
	}
	// this ensures we have a valid wallet if refreshBalance == true
	balance, err := userWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}

	if len(transaction) > 0 {
		// 1. Enforce transaction checks and verify transaction signature
		// NOTE for uphold provider we currently check against user provided publicKey
		//      thus this check does not protect us from a valid fake signature
		txInfo, err := userWallet.VerifyAnonCardTransaction(transaction)
		if err != nil {
			return nil, err
		}
		if txInfo.Probi.LessThan(altcurrency.BAT.ToProbi(decimal.NewFromFloat(lowerTxLimit))) {
			return nil, fmt.Errorf("included transaction must be for a minimum of %g BAT", lowerTxLimit)
		}
		if txInfo.Probi.GreaterThan(altcurrency.BAT.ToProbi(decimal.NewFromFloat(upperTxLimit))) {
			return nil, fmt.Errorf("included transaction must be for a maxiumum of %g BAT", upperTxLimit)
		}
		txProbi = &txInfo.Probi
	}

	// 3. Sum until value is gt transaction amount
	var grants []Grant
	sumProbi := decimal.New(0, 1)
	for _, grant := range unredeemedGrants {
		if txProbi != nil {
			if sumProbi.GreaterThanOrEqual(*txProbi) {
				break
			}
		}
		if *grant.AltCurrency != altcurrency.BAT {
			return nil, errors.New("all grants must be in BAT")
		}
		sumProbi = sumProbi.Add(grant.Probi)
		grants = append(grants, grant)
	}

	if txProbi != nil && txProbi.GreaterThan(balance.SpendableProbi.Add(sumProbi)) {
		return nil, errors.New("wallet does not have enough funds to cover transaction")
	}

	// should be reasonable since we limit the redeem endpoint to a maximum of 1 simultaneous in-flight request
	ugpBalance, err := grantWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}

	if sumProbi.GreaterThan(ugpBalance.SpendableProbi) {
		safeMode = true
		sentry.CaptureException(
			fmt.Errorf("Hot wallet out of funds: %+v!!!",
				map[string]string{"out-of-funds": "true"}))
		return nil, errors.New("ugp wallet lacks enough funds to fulfill grants")
	}

	if len(transaction) > 0 && testSubmit {
		var submitInfo *wallet.TransactionInfo
		// TODO remove this once we can retrieve publicKey info from uphold
		// NOTE We check the signature on the included transaction by submitting it but not confirming it
		submitInfo, err = userWallet.SubmitTransaction(transaction, false)
		if err != nil {
			if wallet.IsInvalidSignature(err) {
				return nil, errors.New("the included transaction was signed with the wrong publicKey")
			} else if !wallet.IsInsufficientBalance(err) {
				return nil, errors.New("error while test submitting the included transaction: " + err.Error())
			}
		}
		redeemTxInfo.ID = submitInfo.ID
	}

	// 4. Iterate through grants and check that:
	for _, grant := range grants {
		// the grant is mature
		if time.Now().Unix() < grant.MaturityTimestamp {
			return nil, errors.New("Grant is not yet redeemable as it is immature")
		}

		// the grant is not expired
		if time.Now().Unix() > grant.ExpiryTimestamp {
			return nil, errors.New("Grant is expired")
		}

		err = service.datastore.RedeemGrantForWallet(grant, walletInfo)
		if err != nil {
			return nil, err
		}

		redeemedGrantsCounter.With(prometheus.Labels{"promotionId": grant.PromotionID.String()}).Inc()
	}

	redeemTxInfo.Probi = sumProbi
	redeemTxInfo.Destination = walletInfo.ProviderID
	return &redeemTxInfo, nil
}

// RedeemGrantsResponse includes information about the transaction to settlement and the grant funds used
type RedeemGrantsResponse struct {
	wallet.TransactionInfo
	GrantTotal decimal.Decimal `json:"grantTotal"`
}

// Redeem the grants in the included response
func (service *Service) Redeem(ctx context.Context, req *RedeemGrantsRequest) (*RedeemGrantsResponse, error) {
	grantFulfillmentInfo, err := service.Consume(ctx, req.WalletInfo, req.Transaction)
	if err != nil {
		return nil, err
	}

	if grantFulfillmentInfo == nil {
		return nil, nil
	}

	submitID := grantFulfillmentInfo.ID

	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		log.Ctx(ctx).
			Error().
			Err(err).
			Msgf("Could not get wallet %s from info after successful Consume", req.WalletInfo.ProviderID)
		sentry.CaptureException(
			fmt.Errorf("Could not get wallet after successful Consume: %+v",
				map[string]string{"providerID": req.WalletInfo.ProviderID}))
		return nil, err
	}

	// fund user wallet with probi from grants
	_, err = grantWallet.Transfer(*grantFulfillmentInfo.AltCurrency, grantFulfillmentInfo.Probi, grantFulfillmentInfo.Destination)
	if err != nil {
		log.Ctx(ctx).
			Error().
			Err(err).
			Msgf("Could not fund wallet %s after successful VerifyAndConsume", req.WalletInfo.ProviderID)
		sentry.CaptureException(
			fmt.Errorf(
				"Could not fund wallet after successful VerifyAndConsume: %+v",
				map[string]string{"providerID": req.WalletInfo.ProviderID}))
		return nil, err
	}

	// confirm settlement transaction previously sent to wallet provider
	var settlementInfo *wallet.TransactionInfo
	for tries := 5; tries >= 0; tries-- {
		// NOTE Consume (by way of VerifyTransaction) guards against transactions that seek to exploit parser differences
		// such as including additional fields that are not understood by this wallet provider implementation but may
		// be understood by the upstream wallet provider.
		settlementInfo, err = userWallet.ConfirmTransaction(submitID)
		if err == nil {
			break
		}
	}

	return &RedeemGrantsResponse{TransactionInfo: *settlementInfo, GrantTotal: grantFulfillmentInfo.Probi}, nil
}

// DrainGrantsRequest a request to drain a wallets grains to a linked uphold account
type DrainGrantsRequest struct {
	WalletInfo       wallet.Info `json:"wallet" valid:"required"`
	AnonymousAddress uuid.UUID   `json:"anonymousAddress" valid:"-"`
}

// DrainGrantsResponse includes info about how much grants were drained
type DrainGrantsResponse struct {
	GrantTotal decimal.Decimal `json:"grantTotal"`
}

// Drain the grants for the wallet in the included response
func (service *Service) Drain(ctx context.Context, req *DrainGrantsRequest) (*DrainGrantsResponse, error) {
	grantFulfillmentInfo, err := service.Consume(ctx, req.WalletInfo, "")
	if err != nil {
		return nil, err
	}

	if grantFulfillmentInfo == nil {
		return &DrainGrantsResponse{decimal.Zero}, nil
	}

	// drain probi from grants into user wallet
	_, err = grantWallet.Transfer(*grantFulfillmentInfo.AltCurrency, grantFulfillmentInfo.Probi, req.AnonymousAddress.String())
	if err != nil {
		log.Ctx(ctx).
			Error().
			Err(err).
			Msgf("Could not drain into wallet %s after successful Consume", req.WalletInfo.ProviderID)
		sentry.CaptureException(
			fmt.Errorf("Could not drain into wallet after successful Consume: %+v",
				map[string]string{
					"providerId": req.WalletInfo.ProviderID,
				}))
		return nil, err
	}
	return &DrainGrantsResponse{grantFulfillmentInfo.Probi}, nil
}
