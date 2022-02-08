package settlement

// NOTE it is important to use submit then confirm to avoid the possibility of duplicate transfers
//      due to transient network errors (if retries are enabled)

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	sentry "github.com/getsentry/sentry-go"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

const maxConfirmTries = 5

// Transaction describes a payout transaction from the settlement wallet to a publisher
type Transaction struct {
	AltCurrency      *altcurrency.AltCurrency `json:"altcurrency"`
	Authority        string                   `json:"authority"`
	Amount           decimal.Decimal          `json:"amount"`
	ExchangeFee      decimal.Decimal          `json:"commission"`
	Currency         string                   `json:"currency"`
	Destination      string                   `json:"address"`
	Publisher        string                   `json:"owner"`
	BATPlatformFee   decimal.Decimal          `json:"fees"`
	Probi            decimal.Decimal          `json:"probi"`
	ProviderID       string                   `json:"hash"`
	WalletProvider   string                   `json:"walletProvider"`
	WalletProviderID string                   `json:"walletProviderId"`
	Channel          string                   `json:"publisher"`
	SignedTx         string                   `json:"signedTx"`
	Status           string                   `json:"status"`
	SettlementID     string                   `json:"transactionId" valid:"uuidv4"`
	TransferFee      decimal.Decimal          `json:"fee"`
	Type             string                   `json:"type"`
	ValidUntil       time.Time                `json:"validUntil,omitempty"`
	DocumentID       string                   `json:"documentId,omitempty"`
	Note             string                   `json:"note"`
}

// AntifraudTransaction a transaction object that comes from antifraud
type AntifraudTransaction struct {
	Transaction
	BAT                decimal.Decimal `json:"bat,omitempty"`
	PayoutReportID     string          `json:"payout_report_id,omitempty"`
	WalletProviderInfo string          `json:"wallet_provider_id,omitempty"`
}

// ProviderInfo holds information parsed from the wallet_provider_id
type ProviderInfo struct {
	Establishment string
	Type          string
	ID            string
}

// TransferID generate the transfer id
func (tx Transaction) TransferID() string {
	inputs := []string{
		tx.SettlementID,
		tx.Destination,
		tx.Channel,
	}
	key := strings.Join(inputs, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// Log logs a message
func (tx Transaction) Log() {
	fmt.Println(tx.Destination, tx.Publisher, tx.TransferID(), tx.Channel)
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
func (at AntifraudTransaction) ToTransaction() Transaction {
	t := at.Transaction
	if at.BAT.GreaterThan(decimal.Zero) {
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
	return t
}

// State contains the current state of the settlement, including wallet and transaction status
type State struct {
	WalletInfo   wallet.Info   `json:"walletInfo"`
	Transactions []Transaction `json:"transactions"`
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

// IsComplete returns true if the transaction status is completed
func (tx Transaction) IsComplete() bool {
	return tx.Status == "completed"
}

// IsFailed returns true if the transaction status is failed
func (tx Transaction) IsFailed() bool {
	return tx.Status == "failed"
}

// PrepareTransactions by embedding signed transactions into the settlement documents
func PrepareTransactions(wallet *uphold.Wallet, settlements []Transaction, purpose string, beneficiary *uphold.Beneficiary) error {
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

func checkTransactionAgainstSettlement(settlement *Transaction, txInfo *wallet.TransactionInfo) error {
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
func CheckPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []Transaction) error {
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
func SubmitPreparedTransaction(ctx context.Context, settlementWallet *uphold.Wallet, settlement *Transaction) error {
	if settlement.IsComplete() {
		fmt.Printf("already complete, skipping submit for channel %s\n", settlement.Channel)
		return nil
	}
	if settlement.IsFailed() {
		fmt.Printf("already failed, skipping submit for channel %s\n", settlement.Channel)
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
				fmt.Printf("transaction already complete for channel %s\n", settlement.Channel)
				return nil
			}
		} else if errorutils.IsErrNotFound(err) { // unconfirmed transactions appear as "not found"
			if time.Now().UTC().Before(settlement.ValidUntil) {
				return nil
			}

			fmt.Printf("already submitted, but quote has expired for channel %s\n", settlement.Channel)
		} else {
			return err
		}
	}

	// post the settlement to uphold but do not confirm it
	submitInfo, err := settlementWallet.SubmitTransaction(ctx, settlement.SignedTx, false)
	if errorutils.IsErrInvalidDestination(err) {
		fmt.Printf("invalid destination, skipping\n")
		settlement.Status = "failed"
		return nil
	} else if err != nil {
		return err
	}

	if time.Now().UTC().Equal(settlement.ValidUntil) || time.Now().UTC().After(settlement.ValidUntil) {
		// BAT transfers have TTL of zero, as do invalid transfers of XAU / LBA
		if submitInfo.DestCurrency == "XAU" || submitInfo.DestCurrency == "LBA" {
			fmt.Printf("quote returned is invalid, skipping\n")
			settlement.Status = "failed"
			return nil
		}
	}

	fmt.Printf("transaction for channel %s submitted, new quote acquired\n", settlement.Channel)

	settlement.ProviderID = submitInfo.ID
	settlement.Status = submitInfo.Status
	settlement.ValidUntil = submitInfo.ValidUntil
	return nil
}

// SubmitPreparedTransactions by submitting them to uphold after performing sanity checks
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be submitted during an initial run can be submitted in subsequent runs.
func SubmitPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []Transaction) error {
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
func ConfirmPreparedTransaction(ctx context.Context, settlementWallet *uphold.Wallet, settlement *Transaction) error {
	for tries := maxConfirmTries; tries >= 0; tries-- {
		if tries == 0 {
			baseMsg := "could not confirm settlement payout after multiple tries: %+v"
			log.Printf("%s for channel %s\n", baseMsg, settlement.Channel)
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
			fmt.Printf("already complete, skipping confirm for channel %s\n", settlement.Channel)
			return nil
		}
		if settlement.IsFailed() {
			fmt.Printf("already failed, skipping confirm for channel %s\n", settlement.Channel)
			return nil
		}

		// first check if the transaction has already been confirmed
		upholdInfo, err := settlementWallet.GetTransaction(ctx, settlement.ProviderID)
		if err == nil {
			settlement.Status = upholdInfo.Status
			settlement.Currency = upholdInfo.DestCurrency
			settlement.Amount = upholdInfo.DestAmount
			settlement.TransferFee = upholdInfo.TransferFee
			settlement.ExchangeFee = upholdInfo.ExchangeFee

			if !settlement.IsComplete() {
				log.Printf("error transaction status is: %s\n", upholdInfo.Status)
			}

			break

		} else if errorutils.IsErrNotFound(err) { // unconfirmed transactions appear as "not found"
			if time.Now().UTC().After(settlement.ValidUntil) {
				log.Printf("quote has expired, must resubmit transaction for channel %s\n", settlement.Channel)
				return nil
			}

			var settlementInfo *wallet.TransactionInfo
			settlementInfo, err = settlementWallet.ConfirmTransaction(ctx, settlement.ProviderID)
			if err == nil {
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
				fmt.Printf("invalid destination, skipping\n")
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
						log.Printf("error transaction status is: %s\n", upholdInfo.Status)
					}
				}
				settlement.Status = "complete"
				break
			} else {
				log.Printf("error confirming: %s\n", err)
			}
		} else {
			log.Printf("error retrieving referenced transaction: %s\n", err)
		}
	}
	return nil
}

// ConfirmPreparedTransactions confirms settlement transactions that have already been submitted to uphold
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be confirmed during an initial run can be confirmed in subsequent runs.
func ConfirmPreparedTransactions(ctx context.Context, settlementWallet *uphold.Wallet, settlements []Transaction) error {
	for i := 0; i < len(settlements); i++ {
		err := ConfirmPreparedTransaction(ctx, settlementWallet, &settlements[i])
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
