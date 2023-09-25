package payments

import "fmt"

// QLDBReocrdNotFoundError indicates that a record does not exist in QLDB.
type QLDBReocrdNotFoundError struct{}

func (e *QLDBReocrdNotFoundError) Error() string {
	return "QLDB record not found"
}

// InvalidTransitionState indicates that a record does not exist in QLDB.
type InvalidTransitionState struct{
	From string
	To string
}

func (e *InvalidTransitionState) Error() string {
	return fmt.Sprintf("invalid state transition from %s to %s.", e.From, e.To)
}

// QLDBTransitionHistoryNotFoundError indicates that an transition history does not exist.
type QLDBTransitionHistoryNotFoundError struct{}

func (e *QLDBTransitionHistoryNotFoundError) Error() string {
	return "QLDB transition history not found"
}

// ErrInvalidVerifier is an error stating the keyID is not a valid verifier.
type ErrInvalidVerifier struct{}

func (e *ErrInvalidVerifier) Error() string {
	return "not a valid verifier"
}

// InsufficientAuthorizationsError indicates that not enough authorizers have submitted
// an authorization to proceed.
type InsufficientAuthorizationsError struct{}

func (e *InsufficientAuthorizationsError) Error() string {
	return "insufficient authorizations"
}
