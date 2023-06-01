package settlement

// NOTE it is important to use submit then confirm to avoid the possibility of duplicate transfers
//      due to transient network errors (if retries are enabled)

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/custodian"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	sentry "github.com/getsentry/sentry-go"
	"github.com/shopspring/decimal"
)

const maxConfirmTries = 5

// AntifraudTransaction is a "v2" transaction, creators only atm
type AntifraudTransaction struct {
	custodian.Transaction
	BAT                decimal.Decimal `json:"bat,omitempty"`
	PayoutReportID     string          `json:"payout_report_id,omitempty"`
	WalletProviderInfo string          `json:"wallet_provider_id,omitempty"`
}

// AggregateTransaction is a single transaction aggregating multiple input transactions
type AggregateTransaction struct {
	custodian.Transaction
	Inputs     []custodian.Transaction `json:"inputs"`
	SourceFrom string                  `json:"source"`
}

// ProviderInfo holds information parsed from the wallet_provider_id
type ProviderInfo struct {
	Establishment string
	Type          string
	ID            string
}

// ProviderInfo splits apart provider info from wallet provider id
func (at AntifraudTransaction) ProviderInfo() ProviderInfo {
	establishmentSplit := strings.Split(at.WalletProviderInfo, "#")
	establishment := establishmentSplit[0]
	typeAndID := establishmentSplit[1]
	typeAndIDSplit := strings.Split(typeAndID, ":")
	return ProviderInfo{
		Establishment: establishment,
		Type:          typeAndIDSplit[0],
		ID:            typeAndIDSplit[1],
	}
}

// ToTransaction turns the antifraud transaction into a transaction understandable by settlement tools
func (at AntifraudTransaction) ToTransaction() (custodian.Transaction, error) {
	t := at.Transaction

	if len(at.Destination) == 0 {
		return t, errors.New("Invalid address")
	}

	if at.BAT.GreaterThan(decimal.Zero) {
		if len(at.WalletProviderInfo) == 0 {
			return t, errors.New("Invalid wallet provider info")

		}
		alt := altcurrency.BAT
		providerInfo := at.ProviderInfo()

		t.Probi = alt.ToProbi(at.BAT)
		t.AltCurrency = &alt
		t.Amount = at.BAT
		t.Currency = alt.String()
		t.BATPlatformFee = alt.ToProbi(at.BATPlatformFee)
		t.WalletProvider = providerInfo.Establishment
		t.WalletProviderID = providerInfo.ID
		t.SettlementID = at.PayoutReportID
	} else if at.Probi.GreaterThan(decimal.Zero) {
		t.Amount = t.AltCurrency.FromProbi(at.Probi)
	}

	if len(t.WalletProviderID) == 0 {
		return t, errors.New("Invalid wallet provider id")
	}

	if !t.Amount.GreaterThan(decimal.NewFromFloat(0)) {
		return t, errors.New("Invalid amount, is not greater than 0")
	}

	return t, nil
}

// State contains the current state of the settlement, including wallet and transaction status
type State struct {
	WalletInfo   wallet.Info             `json:"walletInfo"`
	Transactions []custodian.Transaction `json:"transactions"`
}

// CheckForDuplicates in a list of transactions
func CheckForDuplicates(transactions []AntifraudTransaction) error {
	channelSet := map[string]bool{}
	for _, settlementTransaction := range transactions {
		if _, exists := channelSet[settlementTransaction.Channel]; exists {
			return errors.New("DO NOT PROCEED WITH PAYOUT: Malformed settlement file, duplicate payments detected!:" + settlementTransaction.Channel)
		}
		channelSet[settlementTransaction.Channel] = true
	}
	return nil
}

// PrepareTransactions by embedding signed transactions into the settlement documents
func PrepareTransactions(wallet *uphold.Wallet, settlements []custodian.Transaction, purpose string, beneficiary *uphold.Beneficiary) error {
	for i := 0; i < len(settlements); i++ {
		settlement := &settlements[i]

		// Use the Note field if it exists, otherwise use the settlement ID
		message := settlement.SettlementID
		if len(settlement.Note) > 0 {
			message = settlement.Note
		}
		tx, err := wallet.PrepareTransaction(*settlement.AltCurrency, settlement.Probi, settlement.Destination, message, purpose, beneficiary)
		if err != nil {
			return err
		}
		settlement.SignedTx = tx
	}
	return nil
}

func checkTransactionAgainstSettlement(settlement *custodian.Transaction, txInfo *wallet.TransactionInfo) error {
	if *settlement.AltCurrency != altcurrency.BAT {
		return errors.New("only settlements of BAT are supported")
	}
	// and that the important parts match the rest of the settlement document
	if !settlement.Probi.Equals(txInfo.Probi) {
		return errors.New("embedded signed transaction probi value does not match")
	}
	if len(txInfo.Destination) > 0 && settlement.Destination != txInfo.Destination {
		return errors.New("embedded signed transaction destination address does not match")
	}

	return nil
}

// CheckPreparedTransactions performs sanity checks on an array of signed settlements
func CheckPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []custodian.Transaction) error {
	sumProbi := decimal.Zero
	for i := 0; i < len(settlements); i++ {
		settlement := &settlements[i]

		// make sure the signed transaction is well formed and the signature is valid
		txInfo, err := settlementWallet.VerifyTransaction(ctx, settlement.SignedTx)
		if err != nil {
			return err
		}

		err = checkTransactionAgainstSettlement(settlement, txInfo)
		if err != nil {
			return err
		}

		sumProbi = sumProbi.Add(settlement.Probi)
	}

	// check balance before starting payout
	balance, err := settlementWallet.GetBalance(ctx, true)
	if err != nil {
		return err
	}
	if sumProbi.GreaterThan(balance.SpendableProbi) {
		return errors.New("settlement wallet lacks enough funds to fulfill payout")
	}

	return nil
}

// SubmitPreparedTransaction submits a single settlement transaction to uphold
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be submitted during an initial run can be submitted in subsequent runs.
func SubmitPreparedTransaction(ctx context.Context, settlementWallet *uphold.Wallet, settlement *custodian.Transaction) error {
	logger := logging.Logger(ctx, "settlement.SubmitPreparedTransaction")
	if settlement.IsComplete() {
		logger.Info().Msg(fmt.Sprintf("already complete, skipping submit for channel %s", settlement.Channel))
		return nil
	}
	if settlement.IsFailed() {
		logger.Info().Msg(fmt.Sprintf("already failed, skipping submit for channel %s", settlement.Channel))
		return nil
	}

	if len(settlement.ProviderID) > 0 {
		// first check if the transaction has already been confirmed
		upholdInfo, err := settlementWallet.GetTransaction(ctx, settlement.ProviderID)
		if err == nil {
			settlement.Status = upholdInfo.Status
			settlement.Currency = upholdInfo.DestCurrency
			settlement.Amount = upholdInfo.DestAmount
			settlement.TransferFee = upholdInfo.TransferFee
			settlement.ExchangeFee = upholdInfo.ExchangeFee

			if settlement.IsComplete() {
				logger.Info().Msg(fmt.Sprintf("transaction already complete for channel %s", settlement.Channel))
				return nil
			}
		} else if errorutils.IsErrNotFound(err) { // unconfirmed transactions appear as "not found"
			if time.Now().UTC().Before(settlement.ValidUntil) {
				return nil
			}
			msg := fmt.Sprintf("already submitted, but quote has expired for channel %s", settlement.Channel)
			logger.Info().Msg(msg)
			settlement.FailureReason = msg
		} else {
			return err
		}
	}

	// post the settlement to uphold but do not confirm it
	submitInfo, err := settlementWallet.SubmitTransaction(ctx, settlement.SignedTx, false)
	if errorutils.IsErrInvalidDestination(err) {
		msg := "invalid destination, skipping"
		logger.Info().Msg(msg)
		settlement.Status = "failed"
		settlement.FailureReason = msg
		return nil
	} else if err != nil {
		return err
	}

	if time.Now().UTC().Equal(settlement.ValidUntil) || time.Now().UTC().After(settlement.ValidUntil) {
		// BAT transfers have TTL of zero, as do invalid transfers of XAU / LBA
		if submitInfo.DestCurrency == "XAU" || submitInfo.DestCurrency == "LBA" {
			msg := "quote returned is invalid, skipping"
			logger.Info().Msg(msg)
			settlement.Status = "failed"
			settlement.FailureReason = msg
			return nil
		}
	}

	logger.Info().Msg(fmt.Sprintf("transaction for channel %s submitted, new quote acquired", settlement.Channel))

	settlement.ProviderID = submitInfo.ID
	settlement.Status = submitInfo.Status
	settlement.ValidUntil = submitInfo.ValidUntil
	return nil
}

// SubmitPreparedTransactions by submitting them to uphold after performing sanity checks
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be submitted during an initial run can be submitted in subsequent runs.
func SubmitPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []custodian.Transaction) error {
	err := CheckPreparedTransactions(ctx, settlementWallet, settlements)
	if err != nil {
		return err
	}

	for i := 0; i < len(settlements); i++ {
		err = SubmitPreparedTransaction(ctx, settlementWallet, &settlements[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// ConfirmPreparedTransaction confirms a single settlement transaction with uphold
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be confirmed during an initial run can be submitted in subsequent runs.
func ConfirmPreparedTransaction(
	ctx context.Context,
	settlementWallet *uphold.Wallet,
	settlement *custodian.Transaction,
	isResubmit bool,
) error {
	var (
		settlementInfo *wallet.TransactionInfo
		err            error
	)
	logger := logging.Logger(ctx, "settlement.ConfirmPreparedTransaction")
	for tries := maxConfirmTries; tries >= 0; tries-- {
		if tries == 0 {
			baseMsg := "could not confirm settlement payout after multiple tries: %+v"
			logger.Info().Msg(fmt.Sprintf("%s for channel %s", baseMsg, settlement.Channel))
			sentry.CaptureException(fmt.Errorf(baseMsg, map[string]string{
				"tries":        strconv.Itoa(maxConfirmTries - tries),
				"channel":      settlement.Channel,
				"hash":         settlement.ProviderID,
				"publisher":    settlement.Publisher,
				"settlementId": settlement.SettlementID,
				"status":       settlement.Status,
			}))
		}

		if settlement.IsComplete() {
			logger.Info().Msg(fmt.Sprintf("already complete, skipping confirm for destination %s", settlement.Destination))
			return nil
		}
		if settlement.IsFailed() {
			logger.Info().Msg(fmt.Sprintf("already failed, skipping confirm for destination %s", settlement.Destination))
			return nil
		}

		if isResubmit {
			logger.Info().Msg(fmt.Sprintf("attempting resubmission of transaction for destination: %s", settlement.Destination))
			// first check if the transaction has already been confirmed
			upholdInfo, err := settlementWallet.GetTransaction(ctx, settlement.ProviderID)
			if err == nil {
				settlement.Status = upholdInfo.Status
				settlement.Currency = upholdInfo.DestCurrency
				settlement.Amount = upholdInfo.DestAmount
				settlement.TransferFee = upholdInfo.TransferFee
				settlement.ExchangeFee = upholdInfo.ExchangeFee

				if !settlement.IsComplete() {
					logger.Info().Msg(fmt.Sprintf("error transaction status is: %s", upholdInfo.Status))
				}

				break

			} else if errorutils.IsErrNotFound(err) { // unconfirmed transactions appear as "not found"
				if time.Now().UTC().After(settlement.ValidUntil) {
					logger.Info().Msg(fmt.Sprintf("quote has expired, must resubmit transaction for channel %s", settlement.Channel))
					return nil
				}
			}
		}

		settlementInfo, err = settlementWallet.ConfirmTransaction(ctx, settlement.ProviderID)
		if err == nil {
			logger.Info().Msg(fmt.Sprintf("transaction confirmed for destination: %s", settlement.Destination))
			settlement.Status = settlementInfo.Status
			settlement.Currency = settlementInfo.DestCurrency
			settlement.Amount = settlementInfo.DestAmount
			settlement.TransferFee = settlementInfo.TransferFee
			settlement.ExchangeFee = settlementInfo.ExchangeFee

			// do a sanity check that the uphold transaction confirmed referenced matches the settlement object
			err = checkTransactionAgainstSettlement(settlement, settlementInfo)
			if err != nil {
				return err
			}

			break
		} else if errorutils.IsErrForbidden(err) {
			logger.Error().Err(err).Msg("invalid destination, skipping")
			settlement.Status = "failed"
			return nil
		} else if errorutils.IsErrNotFound(err) {
			logger.Error().Err(err).Msg("transaction not found, skipping")
			settlement.Status = "failed"
			return nil
		} else if errorutils.IsErrAlreadyExists(err) {
			// NOTE we've observed the uphold API LB timing out while the request is eventually processed
			upholdInfo, err := settlementWallet.GetTransaction(ctx, settlement.ProviderID)
			if err == nil {
				settlement.Status = upholdInfo.Status
				settlement.Currency = upholdInfo.DestCurrency
				settlement.Amount = upholdInfo.DestAmount
				settlement.TransferFee = upholdInfo.TransferFee
				settlement.ExchangeFee = upholdInfo.ExchangeFee

				if !settlement.IsComplete() {
					logger.Info().Msg(fmt.Sprintf("error transaction status is: %s", upholdInfo.Status))
				}
			}
			settlement.Status = "complete"
			break
		} else {
			logger.Info().Msg(fmt.Sprintf("error confirming: %s", err))
		}
	}
	return nil
}

// ConfirmPreparedTransactions confirms settlement transactions that have already been submitted to uphold
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be confirmed during an initial run can be confirmed in subsequent runs.
func ConfirmPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []custodian.Transaction) error {
	for i := 0; i < len(settlements); i++ {
		err := ConfirmPreparedTransaction(ctx, settlementWallet, &settlements[i], false)
		if err != nil {
			return err
		}
	}

	return nil
}

// BPTSignedSettlement is a struct describing the signed output format of brave-payment-tools
type BPTSignedSettlement struct {
	SignedTxs []struct {
		uphold.HTTPSignedRequest `json:"signedTx"`
	} `json:"signedTxs"`
}

// ParseBPTSignedSettlement parses the signed output from brave-payment-tools
//   It returns an array of base64 encoded "extracted" httpsignatures
func ParseBPTSignedSettlement(jsonIn []byte) ([]string, error) {
	var s BPTSignedSettlement
	err := json.Unmarshal(jsonIn, &s)
	if err != nil {
		return nil, err
	}
	var encoded []string
	for i := range s.SignedTxs {
		b, err := json.Marshal(s.SignedTxs[i].HTTPSignedRequest)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, base64.StdEncoding.EncodeToString(b))
	}

	return encoded, nil
}
