package middleware

import (
	"context"
	"crypto/sha256"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/requestutils"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// RequestIDTransfer transfers the request id from header to context
func RequestIDTransfer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(requestutils.RequestIDHeaderKey)
		if reqID == "" {
			// generate one if one does not yet exist
			bytes := sha256.Sum256(uuid.NewV4().Bytes())
			reqID = base58.Encode(bytes[:], base58.BitcoinAlphabet)[:16]
		}
		w.Header().Set(requestutils.RequestIDHeaderKey, reqID)
		ctx := context.WithValue(r.Context(), requestutils.RequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TraceIDTransfer transfers the trace id from header baggage to context
func TraceIDTransfer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tracer, propagators, err := appctx.GetOTELTracerPropagatorsFromContext(ctx, "trace-id-middleware")
		if err != nil {
			// bail out if we get an error
			panic(err.Error())
		}

		ctx = propagators.Extract(ctx, propagation.HeaderCarrier(r.Header))
		var span trace.Span
		ctx, span = tracer.Start(ctx, "handle trace id middleware")
		defer span.End()

		// did we get a trace id from the request headers?
		traceID := baggage.FromContext(ctx).Member("traceID").Value()
		if traceID == "" {
			// no, so make a new trace id, if not set should be empty value
			traceID = uuid.NewV4().String()
		}

		// put the trace id on the request context for logging, and later on
		ctx = context.WithValue(ctx, appctx.TraceIDCTXKey, traceID)

		// serve next with our updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
