package logging

import (
	"context"

	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
)

// AddPaymentIDToContext adds payment id to context
func AddPaymentIDToContext(ctx context.Context, paymentID uuid.UUID) {
	l := zerolog.Ctx(ctx)
	if e := l.Debug(); e.Enabled() {
		l.UpdateContext(func(c zerolog.Context) zerolog.Context {
			return c.Str("paymentID", paymentID.String())
		})
	}
}
