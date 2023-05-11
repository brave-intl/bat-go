package middleware

import (
	"context"
	"crypto/sha256"
	"net/http"

	"github.com/brave-intl/bat-go/libs/requestutils"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
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
