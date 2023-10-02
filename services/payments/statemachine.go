package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

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
	transaction      *paymentLib.AuthenticatedPaymentState
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

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Authorize(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	if len(s.getTransaction().Authorizations) >= 2 {
		return s.SetNextState(ctx, paymentLib.Authorized)
	} else {
		return s.transaction, &InsufficientAuthorizationsError{}
	}
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
func StateMachineFromTransaction(
	service *Service,
	authenticatedState *paymentLib.AuthenticatedPaymentState,
) (TxStateMachine, error) {
	var machine TxStateMachine

	switch authenticatedState.PaymentDetails.Custodian {
	case "uphold":
		machine = &UpholdMachine{}
	case "bitflyer":
		// Set Bitflyer-specific properties
		machine = &BitflyerMachine{
			client: *http.DefaultClient,
			bitflyerHost: os.Getenv("BITFLYER_SERVER"),
		}
	case "gemini":
		machine = &GeminiMachine{}
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
	// Drive is called recursively, so we need to check whether a deadline has been set
	// by a prior caller and only set the default deadline if not.
	if _, deadlineSet := ctx.Deadline(); !deadlineSet {
		ctx, _ = context.WithTimeout(ctx, 5 * time.Minute)
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
