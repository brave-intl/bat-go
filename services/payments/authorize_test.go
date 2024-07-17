package payments

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookupVerifier(t *testing.T) {
	a := &Authorizers{}
	_, _, err := a.LookupVerifier(context.Background(), "invalid verifier")

	var verifierError *ErrInvalidAuthorizer
	assert.ErrorAs(t, err, &verifierError,
		"should return an invalid verifier error")
}
