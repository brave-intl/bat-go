package payments

type QLDBReocrdNotFoundError struct{}

func (e *QLDBReocrdNotFoundError) Error() string {
	return "QLDB record not found"
}

// ErrInvalidVerifier is an error stating the keyID is not a valid verifier
type ErrInvalidVerifier struct{}

func (e *ErrInvalidVerifier) Error() string {
	return "not a valid verifier"
}

type InsufficientAuthorizationsError struct{}

func (e *InsufficientAuthorizationsError) Error() string {
	return "insufficient authorizations"
}
