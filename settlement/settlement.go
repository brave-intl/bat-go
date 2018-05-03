package settlement

// NOTE it is important to use submit then confirm to avoid the possibility of duplicate transfers
//      due to transient network errors (if retries are enabled)

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	raven "github.com/getsentry/raven-go"
	"github.com/shopspring/decimal"
)

const maxConfirmTries = 5

// SettlementTransaction describes a payout transaction from the settlement wallet to a publisher
type SettlementTransaction struct {
	AltCurrency    *altcurrency.AltCurrency `json:"altcurrency"`
	Authority      string                   `json:"authority"`
	Amount         decimal.Decimal          `json:"amount"`
	ExchangeFee    decimal.Decimal          `json:"commission"`
	Currency       string                   `json:"currency"`
	Destination    string                   `json:"address"`
	Publisher      string                   `json:"owner"`
	BATPlatformFee decimal.Decimal          `json:"fees"`
	Probi          decimal.Decimal          `json:"probi"`
	ProviderID     string                   `json:"hash" valid:"uuidv4"`
	Channel        string                   `json:"publisher"`
	SignedTx       string                   `json:"signedTx"`
	Status         string                   `json:"status"`
	ID             string                   `json:"transactionId" valid:"uuidv4"`
	TransferFee    decimal.Decimal          `json:"fee"`
	Type           string                   `json:"type"`
	ValidUntil     time.Time                `json:"validUntil"`
}

// Settlement
type SettlementState struct {
	WalletInfo   wallet.Info             `json:"walletInfo"`
	Transactions []SettlementTransaction `json:"transactions"`
}

func (tx SettlementTransaction) IsComplete() bool {
	return tx.Status == "completed"
}

// PrepareSettlementTransactions by embedding signed transactions into the settlement documents
func PrepareSettlementTransactions(wallet *uphold.Wallet, settlements []SettlementTransaction) error {
	for i := 0; i < len(settlements); i++ {
		settlement := &settlements[i]

		tx, err := wallet.PrepareTransaction(*settlement.AltCurrency, settlement.Probi, settlement.Destination, settlement.ID)
		if err != nil {
			return err
		}
		settlement.SignedTx = tx
	}
	return nil
}

func checkTransactionAgainstSettlement(settlement *SettlementTransaction, txInfo *wallet.TransactionInfo) error {
	if *settlement.AltCurrency != altcurrency.BAT {
		return errors.New("only settlements of BAT are supported")
	}
	// and that the important parts match the rest of the settlement document
	if !settlement.Probi.Equals(txInfo.Probi) {
		return errors.New("embedded signed transaction probi value does not match")
	}
	if settlement.Destination != txInfo.Destination {
		return errors.New("embedded signed transaction destination address does not match")
	}

	return nil
}

// CheckPreparedTransactions performs sanity checks on an array of signed settlements
func CheckPreparedTransactions(settlementWallet *uphold.Wallet, settlements []SettlementTransaction) error {
	sumProbi := decimal.Zero
	for i := 0; i < len(settlements); i++ {
		settlement := &settlements[i]

		// make sure the signed transaction is well formed and the signature is valid
		txInfo, err := settlementWallet.VerifyTransaction(settlement.SignedTx)
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
	balance, err := settlementWallet.GetBalance(true)
	if err != nil {
		return err
	}
	if sumProbi.GreaterThan(balance.SpendableProbi) {
		return errors.New("settlement wallet lacks enough funds to fulfill payout")
	}

	return nil
}

func SubmitPreparedTransaction(settlementWallet *uphold.Wallet, settlement *SettlementTransaction) error {
	if settlement.IsComplete() {
		fmt.Printf("already complete, skipping submit for publisher %s\n", settlement.ProviderID)
		return nil
	}

	if len(settlement.ProviderID) > 0 && time.Now().Before(settlement.ValidUntil) {
		fmt.Printf("already submitted, skipping submit for publisher %s\n", settlement.ProviderID)
		return nil
	}

	if len(settlement.ProviderID) > 0 {
		fmt.Printf("already submitted, but quote has expired for publisher %s\n", settlement.ProviderID)
	}

	// post the settlement to uphold but do not confirm it
	submitInfo, err := settlementWallet.SubmitTransaction(settlement.SignedTx, false)
	if err != nil {
		return err
	}

	fmt.Printf("transaction for publisher %s submitted, new quote acquired\n", settlement.ProviderID)

	settlement.ProviderID = submitInfo.ID
	settlement.Status = submitInfo.Status
	settlement.ValidUntil = submitInfo.ValidUntil
	return nil
}

// SubmitPreparedTransactions by submitting them to uphold after performing sanity checks
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be submitted during an initial run can be submitted in subsequent runs.
func SubmitPreparedTransactions(settlementWallet *uphold.Wallet, settlements []SettlementTransaction) error {
	err := CheckPreparedTransactions(settlementWallet, settlements)
	if err != nil {
		return err
	}

	for i := 0; i < len(settlements); i++ {
		err = SubmitPreparedTransaction(settlementWallet, &settlements[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func ConfirmPreparedTransaction(settlementWallet *uphold.Wallet, settlement *SettlementTransaction) error {
	for tries := maxConfirmTries; tries >= 0; tries-- {
		if tries == 0 {
			baseMsg := "could not confirm settlement payout after multiple tries"
			fmt.Fprintf(os.Stderr, "%s for channel %s\n", baseMsg, settlement.Channel)
			raven.CaptureMessage(baseMsg, map[string]string{
				"tries":        strconv.Itoa(maxConfirmTries - tries),
				"channel":      settlement.Channel,
				"hash":         settlement.ProviderID,
				"publisher":    settlement.Publisher,
				"settlementId": settlement.ID,
				"status":       settlement.Status,
			})
		}

		if settlement.IsComplete() {
			fmt.Printf("already complete, skipping confirm for publisher %s\n", settlement.ProviderID)
			return nil
		}

		// first check if the transaction has already been confirmed
		upholdInfo, err := settlementWallet.GetTransaction(settlement.ProviderID)
		if err == nil {
			settlement.Status = upholdInfo.Status
			settlement.Currency = upholdInfo.DestCurrency
			settlement.Amount = upholdInfo.DestAmount
			settlement.TransferFee = upholdInfo.TransferFee
			settlement.ExchangeFee = upholdInfo.ExchangeFee

			if !settlement.IsComplete() {
				fmt.Fprintf(os.Stderr, "error transaction status is: %s\n", upholdInfo.Status)
			}

			break

		} else if wallet.IsNotFound(err) { // unconfirmed transactions appear as "not found"
			if time.Now().After(settlement.ValidUntil) {
				fmt.Printf("quote has expired, must resubmit transaction for publisher %s\n", settlement.ProviderID)
				return nil
			}

			settlementInfo, err := settlementWallet.ConfirmTransaction(settlement.ProviderID)
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
			} else {
				fmt.Fprintf(os.Stderr, "error confirming: %s\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "error retrieving referenced transaction: %s\n", err)
		}
	}
	return nil
}

// ConfirmPreparedTransactions confirms settlement transactions that have already been submitted to uphold
//   It is designed to be idempotent across multiple runs, in case of network outage transactions that
//   were unable to be confirmed during an initial run can be confirmed in subsequent runs.
func ConfirmPreparedTransactions(settlementWallet *uphold.Wallet, settlements []SettlementTransaction) error {
	for i := 0; i < len(settlements); i++ {
		err := ConfirmPreparedTransaction(settlementWallet, &settlements[i])
		if err != nil {
			return err
		}
	}

	return nil
}
