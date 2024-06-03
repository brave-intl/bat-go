package payments

// QLDBReocrdNotFoundError indicates that a record does not exist in QLDB.
type QLDBReocrdNotFoundError struct{}

func (e *QLDBReocrdNotFoundError) Error() string {
	return "QLDB record not found"
}

// QLDBTransitionHistoryNotFoundError indicates that an transition history does not exist.
type QLDBTransitionHistoryNotFoundError struct{}

func (e *QLDBTransitionHistoryNotFoundError) Error() string {
	return "QLDB transition history not found"
}

// ErrInvalidAuthorizer is an error stating the keyID is not a valid payment authorizer.
type ErrInvalidAuthorizer struct{}

func (e *ErrInvalidAuthorizer) Error() string {
	return "not a valid payment authorizer"
}

// InsufficientAuthorizationsError indicates that not enough authorizers have submitted
// an authorization to proceed.
type InsufficientAuthorizationsError struct{}

func (e *InsufficientAuthorizationsError) Error() string {
	return "insufficient authorizations"
}

const (
	SolanaTransactionUnknownError Error = "transaction status unknown"
	SolanaTransactionNotFoundError Error = "transaction not found"
	SolanaTransactionNotConfirmedError Error = "transaction not confirmed"
)

type Error string

func (e Error) Error() string {
	return string(e)
}
