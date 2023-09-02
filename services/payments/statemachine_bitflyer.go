package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/shopspring/decimal"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type BitflyerMachine struct {
	baseStateMachine
	client http.Client
	authToken tokenResponse
	bitflyerHost string
	priceQuote quote
	backoffFactor time.Duration
}

type bitflyerResult struct {
	Transaction *Transaction
	Error error
}

// quote returns a quote of BAT prices
type quote struct {
	ProductCode  string          `json:"product_code"`
	MainCurrency string          `json:"main_currency"`
	SubCurrency  string          `json:"sub_currency"`
	Rate         decimal.Decimal `json:"rate"`
	PriceToken   string          `json:"price_token"`
	ExpiresAt    time.Time
}

// quoteQuery holds the query params for the quote
type quoteQuery struct {
	ProductCode string `url:"product_code,omitempty"`
}

// bitflyerTransactionPayload holds a single withdrawal request
type bitflyerTransactionPayload struct {
	CurrencyCode string       `json:"currency_code"`
	Amount       *ion.Decimal `json:"amount"`
	DryRun       *bool        `json:"dry_run,omitempty"`
	DepositID    string       `json:"deposit_id"`
	TransferID   string       `json:"transfer_id"`
	SourceFrom   string       `json:"source_from"`
}

// bitflyerBulkTransactionPayload holds all WithdrawToDepositIDPayload(s) for a single bulk request
type bitflyerBulkTransactionPayload struct {
	DryRun       bool                         `json:"dry_run"`
	Withdrawals  []bitflyerTransactionPayload `json:"withdrawals"`
	PriceToken   string                       `json:"price_token"`
}

// checkStatusPayload holds the transfer id to check
type checkStatusPayload struct {
	TransferID string `json:"transfer_id"`
}

// checkBulkStatusPayload holds info for checking the status of a transfer
type checkBulkStatusPayload struct {
	Withdrawals []checkStatusPayload `json:"withdrawals"`
}

// bitflyerTransactionResponse holds a single withdrawal request
type bitflyerTransactionResponse struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Message      string          `json:"message"`
	Status       string          `json:"transfer_status"`
	TransferID   string          `json:"transfer_id"`
}

// bitflyerBulkTransactionResponse holds info about the status of the bulk settlements
type bitflyerBulkTransactionResponse struct {
	DryRun      bool                          `json:"dry_run"`
	Withdrawals []bitflyerTransactionResponse `json:"withdrawals"`
}

// tokenPayload holds the data needed to get a new token
type tokenPayload struct {
	GrantType         string `json:"grant_type"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	ExtraClientSecret string `json:"extra_client_secret"`
}

// tokenResponse holds the response from refreshing a token
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	AccountHash  string `json:"account_hash"`
	TokenType    string `json:"token_type"`
	ExpiresAt    time.Time
}

// inventory holds the balance for a particular currency
type inventory struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Available    decimal.Decimal `json:"available"`
}

// inventoryResponse is the response to a balance inquery
type inventoryResponse struct {
	AccountHash string      `json:"account_hash"`
	Inventory   []inventory `json:"inventory"`
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return bm.transaction, err
	}

	// Do nothing if the state is already final
	if bm.transaction.State == Paid || bm.transaction.State == Failed {
		return bm.transaction, nil
	}

	err = bm.refreshToken(ctx, tokenPayload{})
	if err != nil {
		return nil, err
	}
	err = bm.fetchQuote(ctx)
	if err != nil {
		return nil, err
	}
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		entry *Transaction
	)
	batchOfOneTransaction := transactionToBitflyerBulkTransaction(
		bm.transaction,
		bm.priceQuote.PriceToken,
	)
	if bm.transaction.State == Pending {
		// We don't want to check status too fast
		time.Sleep(bm.backoffFactor * time.Millisecond)
		// Get status of transaction and update the state accordingly
		transactionsStatus, err := bm.checkPayoutStatus(ctx, checkBulkStatusPayload{
			Withdrawals: []checkStatusPayload{
				{
					TransferID: batchOfOneTransaction.Withdrawals[0].TransferID,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to check transaction status: %w", err)
		}
		switch strings.ToUpper(transactionsStatus.Withdrawals[0].Status) {
		case "SUCCESS", "EXECUTED":
			// Write the Paid status and end the loop by not calling Drive
			entry, err = bm.writeNextState(ctx, Paid)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
		case "CREATED", "PENDING":
			// Set backoff without changing status
			bm.backoffFactor = bm.backoffFactor * 2
			entry, err = Drive(ctx, bm)
			if err != nil {
				return entry, fmt.Errorf(
					"failed to drive transaction from pending to paid: %w",
					err,
				)
			}
		default:
			// Status unknown. Fail
			entry, err = bm.writeNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from bitflyer for transaction: %v",
				entry,
			)
		}
	} else {
		// Submit formatted transaction
		submittedTransactions, err := bm.uploadBulkPayout(ctx, batchOfOneTransaction)
		if err != nil {
			return nil, fmt.Errorf("failed to submit bulk payout transactions: %w", err)
		}
		if len(submittedTransactions.Withdrawals) != 1 {
			return nil, fmt.Errorf(
				"received an unexpected number of results: %d",
				len(submittedTransactions.Withdrawals),
			)
		}
		// Take the first because it is the only one
		submittedTransaction := submittedTransactions.Withdrawals[0]
		switch strings.ToUpper(submittedTransaction.Status) {
		case "SUCCESS", "EXECUTED":
			// Write the Paid status and end the loop by not calling Drive
			entry, err = bm.writeNextState(ctx, Paid)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
		case "CREATED", "PENDING":
			// Write the Pending status and call Drive to come around again
			entry, err = bm.writeNextState(ctx, Pending)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			// Set initial backoff
			bm.backoffFactor = 2
			entry, err = Drive(ctx, bm)
			if err != nil {
				return entry, fmt.Errorf(
					"failed to drive transaction from pending to paid: %w",
					err,
				)
			}
		default:
			// Status unknown. Fail
			entry, err = bm.writeNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from bitflyer for transaction: %v",
				entry,
			)
		}
	}
	return bm.transaction, nil
}

// Fail implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Failed)
}

func (bm *BitflyerMachine) fetchQuote(
	ctx context.Context,
) (error) {
	if !bm.priceQuote.ExpiresAt.IsZero() && time.Now().Before(bm.priceQuote.ExpiresAt) {
		return nil
	}

	quoteQuery := quoteQuery{
		ProductCode: "BAT_JPY",
	}
	payloadString, err := json.Marshal(quoteQuery)
	if err != nil {
		return fmt.Errorf("failed to parse payload into JSON: %w", err)
	}

	req, err := bm.buildRequest(ctx, bm.bitflyerHost + "/api/link/v1/getprice", "GET", payloadString)
	if err != nil {
		return fmt.Errorf("failed to create price quote request: %w", err)
	}

	resp, err := bm.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get price quote: %w", err)
	}

	var quoteResponse quote
	err = json.NewDecoder(resp.Body).Decode(&quoteResponse)
	if err != nil {
		return fmt.Errorf("failed to parse withdrawal response: %w", err)
	}

	expiresAt, err := parseJWTExpiration(quoteResponse.PriceToken)
	if err != nil {
		return fmt.Errorf("failed to get price quote: %w", err)
	}

	bm.priceQuote.ExpiresAt = expiresAt
	bm.priceQuote = quoteResponse

	return nil
}

// uploadBulkPayout posts a signed bulk layout to bitflyer
func (bm *BitflyerMachine) uploadBulkPayout(
	ctx context.Context,
	payload bitflyerBulkTransactionPayload,
) (*bitflyerBulkTransactionResponse, error) {
	payloadString, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload into JSON: %w", err)
	}
	req, err := bm.buildRequest(
		ctx,
		bm.bitflyerHost + "/api/link/v1/coin/withdraw-to-deposit-id/bulk-request",
		http.MethodPost,
		payloadString,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make withdrawal request: %w", err)
	}

	resp, err := bm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute withdrawal request: %w", err)
	}
	var withdrawToDepositIDBulkResponse bitflyerBulkTransactionResponse
	err = json.NewDecoder(resp.Body).Decode(&withdrawToDepositIDBulkResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse withdrawal response: %w", err)
	}

	return &withdrawToDepositIDBulkResponse, nil
}

// checkPayoutStatus checks the status of a transaction
func (bm *BitflyerMachine) checkPayoutStatus(
	ctx context.Context,
	payload checkBulkStatusPayload,
) (*bitflyerBulkTransactionResponse, error) {
	payloadString, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload into JSON: %w", err)
	}
	req, err := bm.buildRequest(
		ctx,
		bm.bitflyerHost + "/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
		http.MethodPost,
		payloadString,
	)
	if err != nil {
		return nil, err
	}
	resp, err := bm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute status check request: %w", err)
	}
	defer resp.Body.Close()

	var bulkTransactionRespoonse bitflyerBulkTransactionResponse
	err = json.NewDecoder(resp.Body).Decode(&bulkTransactionRespoonse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse status check response: %w", err)
	}
	return &bulkTransactionRespoonse, nil
}

// refreshToken refreshes the token belonging to the provided secret values
func (bm *BitflyerMachine) refreshToken(
	ctx context.Context,
	payload tokenPayload,
) (error) {
	// Only refresh the token if the existing token is defined and has expired
	if time.Now().Before(bm.authToken.ExpiresAt) {
		return nil
	}

	payloadString, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload into JSON: %w", err)
	}
	req, err := bm.buildRequest(
		ctx,
		bm.bitflyerHost + "/api/link/v1/token",
		http.MethodPost,
		payloadString,
	)
	if err != nil {
		return fmt.Errorf("failed to construct token refill request: %w", err)
	}

	resp, err := bm.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute refresh token request: %w", err)
	}
	defer resp.Body.Close()

	var token tokenResponse
	err = json.NewDecoder(resp.Body).Decode(&token)
	if err != nil {
		return fmt.Errorf("failed to parse token refresh response: %w", err)
	}
	bm.authToken = token
	expiresAt, err := time.ParseDuration(fmt.Sprintf("%ds", token.ExpiresIn))
	if err != nil {
		return fmt.Errorf("failed to parse token refresh expires_in: %w", err)
	}
	bm.authToken.ExpiresAt = time.Now().Add(expiresAt)

	return nil
}

func (bm *BitflyerMachine) buildRequest(
	ctx context.Context,
	url,
	method string,
	body []byte,
) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	// If this is the first token refresh call, Bearer will be empty
	req.Header.Set("authorization", "Bearer "+ bm.authToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func transactionToBitflyerBulkTransaction(
	transaction *Transaction,
	priceToken string,
) bitflyerBulkTransactionPayload {
	dryRun := false
	bitflyerTransactions := bitflyerTransactionPayload{
		Amount: transaction.Amount,
		DepositID: transaction.To,
		TransferID: transaction.PayoutID,
		SourceFrom: transaction.From,
		DryRun: &dryRun,
	}
	aggregateTransaction := bitflyerBulkTransactionPayload{
		Withdrawals:      []bitflyerTransactionPayload{bitflyerTransactions},
		PriceToken: priceToken,
		DryRun: false,
	}

	return aggregateTransaction
}

func parseJWTExpiration(token string) (time.Time, error) {
	var claims map[string]interface{}
	parsed, err := jwt.ParseSigned(token)
	if err != nil {
		return time.Time{}, err
	}
	err = parsed.UnsafeClaimsWithoutVerification(&claims)
	if err != nil {
		return time.Time{}, err
	}
	exp := claims["exp"].(float64)
	ts := time.Unix(int64(exp), 0)
	return ts, nil
}
