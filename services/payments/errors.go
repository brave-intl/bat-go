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

// SolanaTransactionNotConfirmedError indicates that a Solana transaction is in any known status
// preceding confirmation on the Solana chain.
type SolanaTransactionNotConfirmedError struct{}

func (e *SolanaTransactionNotConfirmedError) Error() string {
	return "transaction not confirmed"
}

// SolanaTransactionNotFoundError indicates that a Solana transaction was not found on the chain
type SolanaTransactionNotFoundError struct{}

func (e *SolanaTransactionNotFoundError) Error() string {
	return "transaction not found"
}

// SolanaTransactionUnknownError indicates that a response was received but no known status could
// be derived
type SolanaTransactionUnknownError struct{}

func (e *SolanaTransactionUnknownError) Error() string {
	return "transaction status unknown"
}
