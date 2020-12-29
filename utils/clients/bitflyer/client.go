package bitflyer

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	bitflyersettlement "github.com/brave-intl/bat-go/settlement/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/shopspring/decimal"
)

// PrivateRequestSequence handles the ability to sign a request multiple times
type PrivateRequestSequence struct {
	// the baseline object, corresponds to the signature in the first item
	// must update the nonce before sending otherwise invalid signature will be encountered
	Base       BulkPayoutPayload `json:"base"`
	Signatures []string          `json:"signatures"` // a list of hex encoded singatures
	APIKey     string            `json:"apikey"`     // the api key that corresponds to the checksum server side
	Account    *string           `json:"account,omitempty"`
}

// PayoutPayload contains details about transactions to be confirmed
type PayoutPayload struct {
	TxRef       string          `json:"tx_ref"`
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	Destination string          `json:"destination"`
	Account     *string         `json:"account,omitempty"`
}

// AccountListPayload retrieves all accounts associated with a bitflyer key
type AccountListPayload struct {
	Request string `json:"request"`
	Nonce   int64  `json:"nonce"`
}

// BalancesPayload retrieves all accounts associated with a bitflyer key
type BalancesPayload struct {
	Request string  `json:"request"`
	Nonce   int64   `json:"nonce"`
	Account *string `json:"account,omitempty"`
}

// BulkPayoutPayload the payload to be base64'd
type BulkPayoutPayload struct {
	Request       string          `json:"request"`
	Nonce         int64           `json:"nonce"`
	Payouts       []PayoutPayload `json:"payouts"`
	OauthClientID string          `json:"client_id"`
	Account       *string         `json:"account,omitempty"`
}

// Quote returns a quote of BAT prices
type Quote struct {
	ProductCode  string          `json:"product_code"`
	MainCurrency string          `json:"main_currency"`
	SubCurrency  string          `json:"sub_currency"`
	Rate         decimal.Decimal `json:"rate"`
	PriceToken   string          `json:"price_token"`
}

// QuoteQuery holds the query params for the quote
type QuoteQuery struct {
	ProductCode string `json:"product_code"`
}

// WithdrawalRequest holds a single withdrawal request
type WithdrawalRequest struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	DryRun       *bool           `json:"dry_run"`
	DepositID    string          `json:"deposit_id"`
	Memo         string          `json:"memo"`
	TransferID   string          `json:"transfer_id"`
	SourceFrom   string          `json:"source_from"`
}

// WithdrawToDepositIDBulkRequest holds all WithdrawToDepositID for a single bulk request
type WithdrawToDepositIDBulkRequest struct {
	DryRun      *bool               `json:"dry_run"`
	Withdrawals []WithdrawalRequest `json:"withdrawals"`
	PriceToken  string              `json:"price_token"`
}

// NewWithdrawToDepositIDBulkRequest creates a bulk request
func NewWithdrawToDepositIDBulkRequest(dryRun *bool, priceToken string, withdrawals *[]WithdrawalRequest) WithdrawToDepositIDBulkRequest {
	return WithdrawToDepositIDBulkRequest{
		DryRun:      dryRun,
		PriceToken:  priceToken,
		Withdrawals: *withdrawals,
	}
}

// NewWithdrawRequestFromTxs creates an array of withdrawal requests
func NewWithdrawRequestFromTxs(sourceFrom string, txs *[]settlement.Transaction) *[]WithdrawalRequest {
	withdrawals := []WithdrawalRequest{}
	for _, tx := range *txs {
		withdrawals = append(withdrawals, WithdrawalRequest{
			CurrencyCode: "BAT",
			Amount:       tx.Amount,
			DepositID:    tx.Destination,
			Memo:         tx.Note,
			TransferID:   bitflyersettlement.GenerateTransferID(&tx),
			SourceFrom:   sourceFrom,
		})
	}
	return &withdrawals
}

// PayoutResult contains details about a newly created or fetched issuer
type PayoutResult struct {
	Result      string           `json:"result"` // OK or Error
	TxRef       string           `json:"tx_ref"`
	Amount      *decimal.Decimal `json:"amount"`
	Currency    *string          `json:"currency"`
	Destination *string          `json:"destination"`
	Status      *string          `json:"status"`
	Reason      *string          `json:"reason"`
}

// Balance holds balance info
type Balance struct {
	Type                   string          `json:"type"`
	Currency               string          `json:"currency"`
	Amount                 decimal.Decimal `json:"amount"`
	Available              decimal.Decimal `json:"available"`
	AvailableForWithdrawal decimal.Decimal `json:"availableForWithdrawal"`
}

// Account holds account info
type Account struct {
	Name           string `json:"name"`
	Class          string `json:"account"`
	Type           string `json:"type"`
	CounterpartyID string `json:"counterparty_id"`
	CreatedAt      int64  `json:"created"`
}

// GenerateLog creates a log
func (pr PayoutResult) GenerateLog() string {
	if pr.Result == "OK" {
		return ""
	}
	return strings.Join([]string{pr.Result, pr.TxRef, *pr.Status, *pr.Reason}, ": ")
}

// Client abstracts over the underlying client
type Client interface {
	// FetchQuote gets a quote of BAT to JPY
	FetchQuote(ctx context.Context, productCode string) (*Quote, error)
	// // FetchAccountList requests account information to scope future requests
	// FetchAccountList(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Account, error)
	// // FetchBalances requests balance information for a given account
	// FetchBalances(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]Balance, error)
	// UploadBulkPayout posts a signed bulk layout to bitflyer
	UploadBulkPayout(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (*[]PayoutResult, error)
	// CheckTxStatus checks the status of a transaction
	CheckPayoutStatus(ctx context.Context, APIKey string, clientID string, transferID string) (*PayoutResult, error)
}

// HTTPClient wraps http.Client for interacting with the cbr server
type HTTPClient struct {
	client *clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (Client, error) {
	serverEnvKey := "BITFLYER_SERVER"
	serverURL := os.Getenv(serverEnvKey)
	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}
	proxy := os.Getenv("HTTP_PROXY")
	client, err := clients.NewWithProxy("bitflyer", serverURL, os.Getenv("BITFLYER_TOKEN"), proxy)
	if err != nil {
		return nil, err
	}
	return NewClientWithPrometheus(&HTTPClient{client}, "bitflyer_client"), err
}

func setHeaders(
	req *http.Request,
	APIKey string,
	signer *cryptography.HMACKey,
	payload string,
	submitType string,
) error {
	if APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+APIKey)
	}
	return setPrivateRequestHeaders(
		req,
		APIKey,
		signer,
		payload,
		submitType,
	)
}

func setPrivateRequestHeaders(
	req *http.Request,
	APIKey string,
	signer *cryptography.HMACKey,
	payload string,
	submitType string,
) error {
	// if submitType == "hmac" {
	// 	if signer == nil {
	// 		return errors.New("BITFLYER_SUBMIT_TYPE set to 'hmac' but no signer provided")
	// 	}
	// 	signs := *signer
	// 	// only set if sending an hmac salt
	// 	signature, err := signs.HMACSha384([]byte(payload))
	// 	if err != nil {
	// 		return err
	// 	}
	// 	req.Header.Set("X-BITFLYER-SIGNATURE", hex.EncodeToString(signature))
	// }
	return nil
}

// FetchQuote fetches prices for determining constraints
func (c *HTTPClient) FetchQuote(
	ctx context.Context,
	productCode string,
) (*Quote, error) {
	req, err := c.client.NewRequest(ctx, "GET", "/api/link/v1/getprice", QuoteQuery{
		ProductCode: productCode,
	})
	if err != nil {
		return nil, err
	}
	var body Quote
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, nil
}

// UploadBulkPayout uploads payouts to bitflyer
func (c *HTTPClient) UploadBulkPayout(
	ctx context.Context,
	APIKey string,
	signer cryptography.HMACKey,
	payload string,
) (*Quote, error) {
	req, err := c.client.NewRequest(ctx, "POST", "/api/link/v1/coin/withdraw-to-deposit-id/bulk-request", nil)
	if err != nil {
		return nil, err
	}
	var body Quote
	_, err = c.client.Do(ctx, req, &body)
	if err != nil {
		return nil, err
	}
	return &body, nil
}
