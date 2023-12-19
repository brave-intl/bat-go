package payments

import (
	"context"
	"errors"
	"testing"
)

func TestLookupVerifier(t *testing.T) {
	s := &Service{}
	_, _, err := s.LookupVerifier(context.Background(), "invalid verifier")

	if err == nil {
		t.Error("should return an invalid verifier error")
	}
	var verifierError *ErrInvalidAuthorizer
	if !errors.As(err, &verifierError) {
		t.Error("should return an invalid verifier error")
	}
}
