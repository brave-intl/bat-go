package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/brave-intl/bat-go/libs/payments"
)

// TxStateMachine is anything that be progressed through states by the
// Drive function.
type TxStateMachine interface {
	setTransaction(*AuthenticatedPaymentState)
	getState() PaymentStatus
	getTransaction() *AuthenticatedPaymentState
	Prepare(context.Context) (*AuthenticatedPaymentState, error)
	Authorize(context.Context) (*AuthenticatedPaymentState, error)
	Pay(context.Context) (*AuthenticatedPaymentState, error)
	Fail(context.Context) (*AuthenticatedPaymentState, error)
}

type baseStateMachine struct {
	transaction      *AuthenticatedPaymentState
}

func (s *baseStateMachine) SetNextState(
	ctx context.Context,
	nextState PaymentStatus,
) (*AuthenticatedPaymentState, error) {
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
func (s *baseStateMachine) Prepare(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, Prepared)
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Authorize(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.SetNextState(ctx, Authorized)
}

func (s *baseStateMachine) setTransaction(transaction *AuthenticatedPaymentState) {
	s.transaction = transaction
}

// GetState returns transaction state for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getState() PaymentStatus {
	return s.transaction.Status
}

// GetTransaction returns a full transaction for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getTransaction() *AuthenticatedPaymentState {
	return s.transaction
}

// StateMachineFromTransaction returns a state machine when provided a transaction.
func StateMachineFromTransaction(
	service *Service,
	authenticatedState *AuthenticatedPaymentState,
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
	}
	machine.setTransaction(authenticatedState)
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*AuthenticatedPaymentState, error) {
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
	case Prepared:
		if len(machine.getTransaction().Authorizations) >= 2 {
			return machine.Authorize(ctx)
		} else {
			return nil, &InsufficientAuthorizationsError{}
		}
	case Authorized:
		return machine.Pay(ctx)
	case Pending:
		return machine.Pay(ctx)
	case Paid:
		return machine.Pay(ctx)
	case Failed:
		return machine.Fail(ctx)
	default:
		return nil, errors.New("invalid transition state")
	}
}
