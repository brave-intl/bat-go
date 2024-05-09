package payments

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	solanaClient "github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/brave-intl/bat-go/libs/clients/zebpay"
	"github.com/brave-intl/bat-go/libs/nitro"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// TxStateMachine is anything that be progressed through states by the
// Drive function.
type TxStateMachine interface {
	setTransaction(*paymentLib.AuthenticatedPaymentState)
	getState() paymentLib.PaymentStatus
	getTransaction() *paymentLib.AuthenticatedPaymentState
	Prepare(context.Context) (*paymentLib.AuthenticatedPaymentState, error)
	Authorize(context.Context) (*paymentLib.AuthenticatedPaymentState, error)
	Pay(context.Context) (*paymentLib.AuthenticatedPaymentState, error)
	Fail(context.Context) (*paymentLib.AuthenticatedPaymentState, error)
}

type baseStateMachine struct {
	transaction *paymentLib.AuthenticatedPaymentState
}

func (s *baseStateMachine) SetNextState(
	ctx context.Context,
	nextState paymentLib.PaymentStatus,
) (*paymentLib.AuthenticatedPaymentState, error) {
	if !s.transaction.NextStateValid(nextState) {
		return nil, fmt.Errorf(
			"invalid state transition from %s to %s for transaction %s",
			s.transaction.Status,
			nextState,
			s.transaction.DocumentID,
		)
	}
	s.transaction.Status = nextState
	return s.transaction, nil
}

// Prepare implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Prepare(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, paymentLib.Prepared)
}

// IsAuthorized checks whether the state machine has 2 or more authorization, returning true if so
func (s *baseStateMachine) IsAuthorized(ctx context.Context) bool {
	if len(s.getTransaction().Authorizations) >= 2 {
		return true
	}
	return false
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Authorize(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	if s.IsAuthorized(ctx) {
		return s.SetNextState(ctx, paymentLib.Authorized)
	}
	return s.transaction, &InsufficientAuthorizationsError{}
}

func (s *baseStateMachine) setTransaction(transaction *paymentLib.AuthenticatedPaymentState) {
	s.transaction = transaction
}

// GetState returns transaction state for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getState() paymentLib.PaymentStatus {
	return s.transaction.Status
}

// GetTransaction returns a full transaction for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getTransaction() *paymentLib.AuthenticatedPaymentState {
	return s.transaction
}

// StateMachineFromTransaction returns a state machine when provided a transaction.
func (s *Service) StateMachineFromTransaction(
	ctx context.Context,
	authenticatedState *paymentLib.AuthenticatedPaymentState,
) (TxStateMachine, error) {

	// check if service is configured, if not error
	if !s.AreSecretsLoaded(ctx) {
		return nil, errSecretsNotLoaded
	}

	var machine TxStateMachine

	client := http.Client{
		Transport: nitro.NewProxyRoundTripper(ctx, s.egressAddr).(*http.Transport),
	}

	switch authenticatedState.PaymentDetails.Custodian {
	//case "uphold":
	//	machine = &UpholdMachine{}
	//case "bitflyer":
	// Set Bitflyer-specific properties
	//	machine = &BitflyerMachine{
	//		client:       *http.DefaultClient,
	//		bitflyerHost: os.Getenv("BITFLYER_SERVER"),
	//	}
	//case "gemini":
	//	machine = &GeminiMachine{}
	case "zebpay":
		client, err := zebpay.NewWithHTTPClient(client)
		if err != nil {
			return nil, fmt.Errorf("failed to create zebpay client: %w", err)
		}
		block, rest := pem.Decode([]byte(os.Getenv("ZEBPAY_SIGNING_KEY")))
		if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
			return nil, fmt.Errorf("failed to decode zebpay signing key: %w", err)
		}
		signingKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse zebpay signing key: %w", err)
		}
		machine = &ZebpayMachine{
			client:     client,
			apiKey:     os.Getenv("ZEBPAY_API_KEY"),
			signingKey: signingKey,
			zebpayHost: os.Getenv("ZEBPAY_SERVER"),
		}
	case "solana":
		solClient := solanaClient.New(rpc.WithEndpoint(os.Getenv("SOLANA_RPC_ENDPOINT")), rpc.WithHTTPClient(&client))
		keyBytes, err := base64.StdEncoding.DecodeString(os.Getenv("SOLANA_SIGNING_KEY"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode solana private key: %w", err)
		}
		machine = &SolanaMachine{
			signingKey:      keyBytes,
			solanaRpcClient: *solClient,
			splMintAddress:  SPLBATMintAddress,
			splMintDecimals: SPLBATMintDecimals,
		}
	case "dryrun-happypath":
		machine = &HappyPathMachine{}
	case "dryrun-prepare-fails":
		machine = &PrepareFailsMachine{}
	case "dryrun-authorize-fails":
		machine = &AuthorizeFailsMachine{}
	case "dryrun-pay-fails":
		machine = &PayFailsMachine{}
	}
	machine.setTransaction(authenticatedState)
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*paymentLib.AuthenticatedPaymentState, error) {
	// Check whether a deadline has been set by a prior caller and only set the default deadline if
	// not.
	if _, deadlineSet := ctx.Deadline(); !deadlineSet {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		transaction := machine.getTransaction()
		return transaction, err
	}

	// If the transaction does exist in the database, attempt to drive the state machine forward
	switch machine.getState() {
	case paymentLib.Prepared:
		return machine.Authorize(ctx)
	case paymentLib.Authorized:
		return machine.Pay(ctx)
	case paymentLib.Pending:
		return machine.Pay(ctx)
	case paymentLib.Paid:
		return machine.Pay(ctx)
	case paymentLib.Failed:
		return machine.Fail(ctx)
	default:
		return nil, errors.New("invalid transition state")
	}
}

// DriveTransaction attempts to Drive the Transaction forward.
func (s *Service) DriveTransaction(
	ctx context.Context,
	transaction *paymentLib.AuthenticatedPaymentState,
) error {
	stateMachine, err := s.StateMachineFromTransaction(ctx, transaction)
	if err != nil {
		return fmt.Errorf("failed to create stateMachine: %w", err)
	}

	state, lastErr := Drive(ctx, stateMachine)
	if state != nil {
		var errTmp paymentLib.PaymentError
		if errors.As(lastErr, &errTmp) {
			state.LastError = &errTmp
		} else if lastErr != nil {
			// Assume any non-categorized error is temporary
			state.LastError = paymentLib.ProcessingErrorFromError(lastErr, true)
		} else {
			state.LastError = nil
		}

		marshaledState, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}

		paymentState := paymentLib.PaymentState{
			UnsafePaymentState: marshaledState,
		}

		err = paymentState.Sign(s.signer, s.publicKey)
		if err != nil {
			return fmt.Errorf("failed to sign transaction: %w", err)
		}

		s.datastore.UpdatePaymentState(ctx, state.DocumentID, &paymentState)

		if err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}

		if lastErr != nil {
			// Insufficient authorizations is an expected state. Treat it as such.
			var errTmp *InsufficientAuthorizationsError
			if !errors.As(lastErr, &errTmp) {
				return fmt.Errorf("failed to progress transaction: %w", lastErr)
			}
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("failed to progress transaction: %w", lastErr)
	}
	return errors.New("failed to progress transaction, no state returned")
}
