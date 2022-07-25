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
	if !errors.Is(err, ErrInvalidVerifier) {
		t.Error("should return an invalid verifier error")
	}
}
