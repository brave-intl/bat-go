package payments

import (
	"context"
	"fmt"
	"time"
	"net/http"
	"bytes"
	//"io"
	"encoding/json"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	//"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/tools/settlement"
	//bitflyersettlement "github.com/brave-intl/bat-go/tools/settlement/bitflyer"
	//bitflyercmd "github.com/brave-intl/bat-go/tools/settlement/cmd"
	"github.com/shopspring/decimal"
)

// quote returns a quote of BAT prices
type quote struct {
	ProductCode  string          `json:"product_code"`
	MainCurrency string          `json:"main_currency"`
	SubCurrency  string          `json:"sub_currency"`
	Rate         decimal.Decimal `json:"rate"`
	PriceToken   string          `json:"price_token"`
}

// quoteQuery holds the query params for the quote
type quoteQuery struct {
	ProductCode string `url:"product_code,omitempty"`
}

// withdrawToDepositIDPayload holds a single withdrawal request
type withdrawToDepositIDPayload struct {
	CurrencyCode string  `json:"currency_code"`
	Amount       float64 `json:"amount"`
	DryRun       *bool   `json:"dry_run,omitempty"`
	DepositID    string  `json:"deposit_id"`
	TransferID   string  `json:"transfer_id"`
	SourceFrom   string  `json:"source_from"`
}

// withdrawToDepositIDBulkPayload holds all WithdrawToDepositIDPayload(s) for a single bulk request
type withdrawToDepositIDBulkPayload struct {
	DryRun       bool                         `json:"dry_run"`
	Withdrawals  []withdrawToDepositIDPayload `json:"withdrawals"`
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

// withdrawToDepositIDResponse holds a single withdrawal request
type withdrawToDepositIDResponse struct {
	CurrencyCode string          `json:"currency_code"`
	Amount       decimal.Decimal `json:"amount"`
	Message      string          `json:"message"`
	Status       string          `json:"transfer_status"`
	TransferID   string          `json:"transfer_id"`
}

// withdrawToDepositIDBulkResponse holds info about the status of the bulk settlements
type withdrawToDepositIDBulkResponse struct {
	DryRun      bool                          `json:"dry_run"`
	Withdrawals []withdrawToDepositIDResponse `json:"withdrawals"`
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

// categorizeStatus checks the status of a withdrawal response and categorizes it
//func categorizeStatus(withdrawResponse withdrawToDepositIDResponse) string {
//	switch withdrawResponse.Status {
//	case "SUCCESS", "EXECUTED":
//		return "complete"
//	case "CREATED", "PENDING":
//		return "pending"
//	}
//	return "failed"
//}
//
//func fetchQuote(
//	ctx context.Context,
//	productCode string,
//	readFromFile bool,
//) (*quote, error) {
//
//}
//
// uploadBulkPayout posts a signed bulk layout to bitflyer
func (bm *BitflyerMachine) uploadBulkPayout(
	ctx context.Context,
	payload withdrawToDepositIDBulkPayload,
) (*withdrawToDepositIDBulkResponse, error) {
	payloadString, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload into JSON: %w", err)
	}
	req, err := bm.makeRequest(
		ctx,
		http.MethodPost,
		"/api/link/v1/coin/withdraw-to-deposit-id/bulk-request",
		payloadString,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to make withdrawal request: %w", err)
	}

	resp, err := bm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute withdrawal request: %w", err)
	}
	var withdrawToDepositIDBulkResponse withdrawToDepositIDBulkResponse
	err = json.NewDecoder(resp.Body).Decode(&withdrawToDepositIDBulkResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse withdrawal response: %w", err)
	}

	return &withdrawToDepositIDBulkResponse, nil
}
//
//// checkPayoutStatus checks the status of a transaction
//func checkPayoutStatus(
//	ctx context.Context,
//	payload checkBulkStatusPayload,
//) (*withdrawToDepositIDBulkResponse, error) {
//
//}
//
//// checkInventory check available balance of bitflyer account
//func checkInventory(ctx context.Context) (map[string]inventory, error) {
//
//}

// refreshToken refreshes the token belonging to the provided secret values
func (bm *BitflyerMachine) refreshToken(
	ctx context.Context,
	payload tokenPayload,
) (error) {
	payloadString, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload into JSON: %w", err)
	}
	req, err := bm.makeRequest(
		ctx,
		"http://bravesoftware.com/api/link/v1/token",
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
	fmt.Printf("TOKEN: %#v\n", token)

	return nil
}

// fetchBalance requests balance information for the auth token on the underlying client object
//func fetchBalance(ctx context.Context) (*inventoryResponse, error) {
//
//}

func (bm *BitflyerMachine) makeRequest(
	ctx context.Context,
	url,
	method string,
	body []byte,
) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	// If this is the first token refresh call, Bearer will be empty
	req.Header.Set("authorization", "Bearer "+ bm.authToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type BitflyerMachine struct {
	baseStateMachine
	client http.Client
	authToken tokenResponse
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	fmt.Println("refreshing token")
	err := bm.refreshToken(ctx, tokenPayload{})
	if err != nil {
		return nil, err
	}
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		entry                 *Transaction
		//submittedTransactions map[string][]custodian.Transaction
	)
	if bm.transaction.State == Pending {
		// Get status of transaction and update the state accordingly
		return bm.writeNextState(ctx, Paid)
	} else {
		// Submit formatted transaction
		aggregateTransaction := transactionToSettlementAggregateTransaction(bm.transaction)
		aggregateTransactionSet := []settlement.AggregateTransaction{aggregateTransaction}
		request, err := getBitflyerRequest(ctx, bitflyerClient, aggregateTransactionSet)
		if err != nil {
			return nil, fmt.Errorf("failed to get bitflyer request: %w", err)
		}
		submittedTransactions, err = bitflyersettlement.SubmitBulkPayoutTransactions(
			ctx,
			aggregateTransactionSet,
			submittedTransactions,
			*request,
			bitflyerClient,
			1, // Hard code number of transactions, as we will only do one at a time
			1, // Hard code progress, as there is only one
		)
		if err != nil {
			return nil, fmt.Errorf("failed to submit bulk payout transactions: %w", err)
		}
		entry, err = bm.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
		entry, err = Drive(ctx, bm)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	return entry, nil
}

// Fail implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Failed)
}

//func getBitflyerRequest(
//	ctx context.Context,
//	bitflyerClient bitflyer.Client,
//	aggregateTransactions []settlement.AggregateTransaction,
//) (*withdrawToDepositIDBulkPayload, error) {
//	//  this will only fetch a new quote when needed - but ensures that we don't have problems due
//	//  to quote expiring midway through
//	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
//	if err != nil {
//		return nil, fmt.Errorf("failed to fetch bitflyer quote: %w", err)
//	}
//
//	request, err := bitflyersettlement.CreateBitflyerRequest(
//		nil, // Ignore dry run settings for now. We handle it ourselves.
//		quote.PriceToken,
//		aggregateTransactions,
//	)
//	if err != nil {
//		return nil, err
//	}
//	return request, nil
//}

func transactionToSettlementAggregateTransaction(transaction *Transaction) settlement.AggregateTransaction {
	altCurrencyBAT := altcurrency.BAT
	custodianTransaction := custodian.Transaction{
		AltCurrency:      &altCurrencyBAT,
		Authority:        "",
		Amount:           decimal.New(1, 1),
		ExchangeFee:      decimal.New(1, 1),
		FailureReason:    "",
		Currency:         "",
		Destination:      "",
		Publisher:        "",
		BATPlatformFee:   decimal.New(1, 1),
		Probi:            decimal.New(1, 1),
		ProviderID:       "",
		WalletProvider:   "",
		WalletProviderID: "",
		Channel:          "",
		SignedTx:         "",
		Status:           "",
		SettlementID:     "",
		TransferFee:      decimal.New(1, 1),
		Type:             "",
		ValidUntil:       time.Now(),
		DocumentID:       "",
		Note:             "",
	}
	aggregateTransaction := settlement.AggregateTransaction{
		Transaction: custodianTransaction,
		Inputs:      []custodian.Transaction{custodianTransaction},
		SourceFrom:  "",
	}
	return aggregateTransaction
}
