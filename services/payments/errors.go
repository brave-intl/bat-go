package payments

import (
	"fmt"
)

type QLDBReocrdNotFoundError struct{}

func (e *QLDBReocrdNotFoundError) Error() string {
	return fmt.Sprintf("QLDB record not found")
}

// ErrInvalidVerifier is an error stating the keyID is not a valid verifier
type ErrInvalidVerifier struct{}

func (e *ErrInvalidVerifier) Error() string {
	return fmt.Sprint("not a valid verifier")
}

type InsufficientAuthorizationsError struct{}

func (e *InsufficientAuthorizationsError) Error() string {
	return fmt.Sprintf("insufficient authorizations")
}
