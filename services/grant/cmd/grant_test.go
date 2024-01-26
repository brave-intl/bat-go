package cmd

import (
	"context"
	"testing"

	should "github.com/stretchr/testify/assert"

	appctx "github.com/brave-intl/bat-go/libs/context"
)

func TestNewSrvStatusFromCtx(t *testing.T) {
	ctx := context.TODO()

	ctx = context.WithValue(ctx, appctx.DisableUpholdLinkingCTXKey, true)
	ctx = context.WithValue(ctx, appctx.DisableGeminiLinkingCTXKey, true)
	ctx = context.WithValue(ctx, appctx.DisableBitflyerLinkingCTXKey, true)
	ctx = context.WithValue(ctx, appctx.DisableZebPayLinkingCTXKey, true)
	ctx = context.WithValue(ctx, appctx.DisableSolanaLinkingCTXKey, true)

	act := newSrvStatusFromCtx(ctx)
	exp := map[string]interface{}{
		"wallet": map[string]bool{
			"uphold":   false,
			"gemini":   false,
			"bitflyer": false,
			"zebpay":   false,
			"solana":   false,
		},
	}

	should.Equal(t, exp, act)
}
