package payments

import "fmt"

type QLDBReocrdNotFoundError struct{}

func (e *QLDBReocrdNotFoundError) Error() string {
	return fmt.Sprintf("QLDB record not found")
}

type InsufficientAuthorizationsError struct{}

func (e *InsufficientAuthorizationsError) Error() string {
	return fmt.Sprintf("insufficient authorizations")
}
