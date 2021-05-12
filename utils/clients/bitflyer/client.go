package bitflyer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/google/go-querystring/query"
	"github.com/shopspring/decimal"
	"github.com/square/go-jose/jwt"
)

var (
	validSourceFrom = map[string]bool{
		"tipping":   true,
		"adrewards": true,
		"userdrain": true,
	}
)

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
	ProductCode string `url:"product_code,omitempty"`
}

// GenerateQueryString - implement the QueryStringBody interface
func (qq *QuoteQuery) GenerateQueryString() (url.Values, error) {
	return query.Values(qq)
}

// WithdrawToDepositIDPayload holds a single withdrawal request
type WithdrawToDepositIDPayload struct {
	CurrencyCode string  `json:"currency_code"`
	Amount       float64 `json:"amount"`
	DryRun       *bool   `json:"dry_run,omitempty"`
	DepositID    string  `json:"deposit_id"`
	TransferID   string  `json:"transfer_id"`
	SourceFrom   string  `json:"source_from"`
}

// WithdrawToDepositIDBulkPayload holds all WithdrawToDepositIDPayload(s) for a single bulk request
type WithdrawToDepositIDBulkPayload struct {
	DryRun       bool                         `json:"dry_run"`
	Withdrawals  []WithdrawToDepositIDPayload `json:"withdrawals"`
	PriceToken   string                       `json:"price_token"`
	DryRunOption *DryRunOption                `json:"dry_run_option,omitempty"`
}

// CheckStatusPayload holds the transfer id to check
type CheckStatusPayload struct {
	TransferID string `json:"transfer_id"`
}

// CheckBulkStatusPayload holds info for checking the status of a transfer
type CheckBulkStatusPayload struct {
	Withdrawals []CheckStatusPayload `json:"withdrawals"`
}

// TransferIDsToBulkStatus takes a list of transferIDs and turns them into a payload for checking their status
func TransferIDsToBulkStatus(transferIDs []string) CheckBulkStatusPayload {
	checkStatusPayload := []CheckStatusPayload{}
	for _, transferID := range transferIDs {
		checkStatusPayload = append(checkStatusPayload, CheckStatusPayload{
			TransferID: transferID,
		})
	}
	return CheckBulkStatusPayload{
		Withdrawals: checkStatusPayload,
	}
}

// ToBulkStatus converts an upload to a checks status payload
func (w WithdrawToDepositIDBulkPayload) ToBulkStatus() CheckBulkStatusPayload {
	checkStatusPayload := []CheckStatusPayload{}
	for _, wd := range w.Withdrawals {
		checkStatusPayload = append(checkStatusPayload, CheckStatusPayload{
			TransferID: wd.TransferID,
		})
	}
	return CheckBulkStatusPayload{
		Withdrawals: checkStatusPayload,
	}
}

// WithdrawToDepositIDResponse holds a single withdrawal request
type WithdrawToDepositIDResponse struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Message      string          `json:"message"`
	Status       string          `json:"transfer_status"`
	TransferID   string          `json:"transfer_id"`
}

// CategorizeStatus checks the status of a withdrawal response and categorizes it
func (withdrawResponse WithdrawToDepositIDResponse) CategorizeStatus() string {
	switch withdrawResponse.Status {
	case "SUCCESS", "EXECUTED":
		return "complete"
	case "NOT_FOUND", "NO_INV", "INVALID_MEMO", "NOT_FOUNTD", "INVALID_AMOUNT", "NOT_ALLOWED_TO_SEND", "NOT_ALLOWED_TO_RECV", "LOCKED_BY_QUICK_DEPOSIT", "SESSION_SEND_LIMIT", "SESSION_TIME_OUT", "EXPIRED", "NOPOSITION", "OTHER_ERROR", "MONTHLY_SEND_LIMIT":
		return "failed"
	case "CREATED", "PENDING":
		return "pending"
	}
	return "unknown"
}

// TokenPayload holds the data needed to get a new token
type TokenPayload struct {
	GrantType         string `json:"grant_type"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	ExtraClientSecret string `json:"extra_client_secret"`
}

// TokenResponse holds the response from refreshing a token
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccountHash  string `json:"account_hash"`
	TokenType    string `json:"token_type"`
}

// DryRunOption holds options for dry running a transaction
type DryRunOption struct {
	RequestAPITransferStatus string `json:"request_api_transfer_status"`
	ProcessTimeSec           uint   `json:"process_time_sec"`
	StatusAPITransferStatus  string `json:"status_api_transfer_status"`
}

// NewWithdrawToDepositIDBulkPayload creates a bulk request
func NewWithdrawToDepositIDBulkPayload(dryRunOptions *DryRunOption, priceToken string, withdrawals *[]WithdrawToDepositIDPayload) *WithdrawToDepositIDBulkPayload {
	dryRun := false
	if dryRunOptions != nil {
		dryRun = true
		enum := "ENUM"
		if dryRunOptions.RequestAPITransferStatus != enum {
			dryRunOptions.RequestAPITransferStatus = enum
		}
		if dryRunOptions.StatusAPITransferStatus != enum {
			dryRunOptions.StatusAPITransferStatus = enum
		}
	}
	return &WithdrawToDepositIDBulkPayload{
		PriceToken:   priceToken,
		Withdrawals:  *withdrawals,
		DryRun:       dryRun,
		DryRunOption: dryRunOptions,
	}
}

// WithdrawToDepositIDBulkResponse holds info about the status of the bulk settlements
type WithdrawToDepositIDBulkResponse struct {
	DryRun      bool                          `json:"dry_run"`
	Withdrawals []WithdrawToDepositIDResponse `json:"withdrawals"`
}

// NewWithdrawsFromTxs creates an array of withdrawal requests
func NewWithdrawsFromTxs(
	sourceFrom string,
	txs []settlement.Transaction,
) (*[]WithdrawToDepositIDPayload, error) {
	withdrawals := []WithdrawToDepositIDPayload{}
	if !validSourceFrom[sourceFrom] {
		return nil, fmt.Errorf("valid `sourceFrom` value must be passed got: `%s`", sourceFrom)
	}
	for _, tx := range txs {
		bat := altcurrency.BAT.FromProbi(tx.Probi)
		if bat.Exponent() > 8 {
			return nil, fmt.Errorf("cannot convert float exactly, %d", bat)
		}
		// exact is never true, equality check needed
		f64, _ := bat.Float64()
		if !decimal.NewFromFloat(f64).Equal(bat) {
			return nil, fmt.Errorf("bat conversion did not work: %.8f is not equal %d", f64, bat)
		}
		withdrawals = append(withdrawals, WithdrawToDepositIDPayload{
			CurrencyCode: "BAT",
			Amount:       f64,
			DepositID:    tx.Destination,
			TransferID:   tx.TransferID(),
			SourceFrom:   sourceFrom,
		})
	}
	return &withdrawals, nil
}

// Client abstracts over the underlying client
type Client interface {
	// FetchQuote gets a quote of BAT to JPY
	FetchQuote(ctx context.Context, productCode string, readFromFile bool) (*Quote, error)
	// UploadBulkPayout posts a signed bulk layout to bitflyer
	UploadBulkPayout(ctx context.Context, payload WithdrawToDepositIDBulkPayload) (*WithdrawToDepositIDBulkResponse, error)
	// CheckPayoutStatus checks the status of a transaction
	CheckPayoutStatus(ctx context.Context, payload CheckBulkStatusPayload) (*WithdrawToDepositIDBulkResponse, error)
	// RefreshToken refreshes the token belonging to the provided secret values
	RefreshToken(ctx context.Context, payload TokenPayload) (*TokenResponse, error)
	// SetAuthToken sets the auth token on underlying client object
	SetAuthToken(authToken string)
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

// SetAuthToken sets the auth token
func (c *HTTPClient) SetAuthToken(
	authToken string,
) {
	c.client.AuthToken = authToken
}

// FetchQuote fetches prices for determining constraints
func (c *HTTPClient) FetchQuote(
	ctx context.Context,
	productCode string,
	readFromFile bool,
) (*Quote, error) {
	if readFromFile {
		read, err := readQuoteFromFile()
		if err != nil {
			fmt.Println("failed to read quote from file", err)
			return nil, err
		}
		if withinPriceTokenExpiration(read) {
			return &read.Body, nil
		}
	}
	req, err := c.client.NewRequest(ctx, "GET", "/api/link/v1/getprice", nil, &QuoteQuery{
		ProductCode: productCode,
	})
	if err != nil {
		return nil, err
	}

	// use the client auth token, token is required for bf api call
	c.setupRequestHeaders(req)

	var body Quote
	resp, err := c.client.Do(ctx, req, &body)
	if err == nil {
		expiry, err := parseExpiry(body.PriceToken)
		if err == nil {
			writeQuoteToFile(SavedQuote{
				Body:   body,
				Expiry: *expiry,
			})
		}
	}
	return &body, handleBitflyerError(err, req, resp)
}

// PriceTokenInfo holds info from the price token
type PriceTokenInfo struct {
	ProductCode string          `json:"product_code,omitempty"`
	Rate        decimal.Decimal `json:"rate,omitempty"`
	IssuedAt    int             `json:"iat,omitempty"`
	Expiry      int             `json:"exp,omitempty"`
}

func parseExpiry(token string) (*time.Time, error) {
	var claims map[string]interface{}
	parsed, err := jwt.ParseSigned(token)
	if err != nil {
		return nil, err
	}
	err = parsed.UnsafeClaimsWithoutVerification(&claims)
	if err != nil {
		return nil, err
	}
	exp := claims["exp"].(float64)
	ts := time.Unix(int64(exp), 0)
	return &ts, nil
}

func withinPriceTokenExpiration(savedQuote *SavedQuote) bool {
	if savedQuote == nil {
		return false
	}
	return time.Now().Before(savedQuote.Expiry)
}

func writeQuoteToFile(quote SavedQuote) {
	data, err := json.Marshal(quote)
	if err != nil {
		fmt.Println("marshal error", err)
		return
	}
	_ = ioutil.WriteFile("./fetch-quote.json", data, 0777)
}

// SavedQuote stores a quote locally
type SavedQuote struct {
	Body   Quote     `json:"body"`
	Expiry time.Time `json:"expiry"`
}

func readQuoteFromFile() (*SavedQuote, error) {
	dat, err := ioutil.ReadFile("./fetch-quote.json")
	if err != nil {
		fmt.Println("read file error", err)
		return nil, nil
	}
	var body SavedQuote
	err = json.Unmarshal(dat, &body)
	if err != nil {
		return nil, fmt.Errorf("unmarshal quote file error: %w", err)
	}
	return &body, nil
}

// UploadBulkPayout uploads payouts to bitflyer
func (c *HTTPClient) UploadBulkPayout(
	ctx context.Context,
	payload WithdrawToDepositIDBulkPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/coin/withdraw-to-deposit-id/bulk-request", payload, nil)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body WithdrawToDepositIDBulkResponse
	resp, err := c.client.Do(ctx, req, &body)
	return &body, handleBitflyerError(err, req, resp)
}

// CheckPayoutStatus checks bitflyer transaction status
func (c *HTTPClient) CheckPayoutStatus(
	ctx context.Context,
	payload CheckBulkStatusPayload,
) (*WithdrawToDepositIDBulkResponse, error) {
	req, err := c.client.NewRequest(
		ctx,
		http.MethodPost,
		"/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
		payload,
		nil,
	)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body WithdrawToDepositIDBulkResponse
	resp, err := c.client.Do(ctx, req, &body)
	return &body, handleBitflyerError(err, req, resp)
}

// RefreshToken gets a new token from bitflyer
func (c *HTTPClient) RefreshToken(
	ctx context.Context,
	payload TokenPayload,
) (*TokenResponse, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logger.Info().
		Str("client_id", payload.ClientID).
		Str("client_secret", payload.ClientSecret).
		Str("extra_client_secret", payload.ExtraClientSecret).
		Str("grant_type", payload.GrantType).
		Msg("payload values")
	req, err := c.client.NewRequest(ctx, http.MethodPost, "/api/link/v1/token", payload, nil)
	if err != nil {
		return nil, err
	}
	c.setupRequestHeaders(req)
	var body TokenResponse
	resp, err := c.client.Do(ctx, req, &body)
	logger.Info().
		Str("token", body.AccessToken).
		Msg("using updated token. make sure this value is in your env vars (BITFLYER_TOKEN) to avoid refreshes")
	c.SetAuthToken(body.AccessToken)
	return &body, handleBitflyerError(err, req, resp)
}

func (c *HTTPClient) setupRequestHeaders(req *http.Request) {
	req.Header.Set("authorization", "Bearer "+c.client.AuthToken)
	req.Header.Set("content-type", "application/json")
}

func handleBitflyerError(e error, req *http.Request, resp *http.Response) error {
	if resp == nil {
		return e
	}
	b, err := requestutils.Read(resp.Body)
	if err != nil {
		return err
	}
	var bfError clients.BitflyerError
	if len(b) != 0 {
		err = json.Unmarshal(b, &bfError)
		if err != nil {
			return err
		}
	}
	if len(bfError.Label) == 0 {
		return e
	}
	return bfError
}

// TokenPayloadFromCtx - given some context, create our bf token payload
func TokenPayloadFromCtx(ctx context.Context) TokenPayload {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}
	// get bf creds from context
	clientID, err := appctx.GetStringFromContext(ctx, appctx.BitflyerClientIDCTXKey)
	if err != nil {
		// misconfigured, needs client id
		logger.Error().Err(err).Msg("missing bitflyer client id from ctx")
	}
	clientSecret, err := appctx.GetStringFromContext(ctx, appctx.BitflyerClientSecretCTXKey)
	if err != nil {
		// misconfigured, needs client Secret
		logger.Error().Err(err).Msg("missing bitflyer client Secret from ctx")
	}
	extraClientSecret, err := appctx.GetStringFromContext(ctx, appctx.BitflyerExtraClientSecretCTXKey)
	if err != nil {
		// misconfigured, needs extra client secret
		logger.Error().Err(err).Msg("missing bitflyer extra client Secret from ctx")
	}
	return TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		ExtraClientSecret: extraClientSecret,
	}
}
