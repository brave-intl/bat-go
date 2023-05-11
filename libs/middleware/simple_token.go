package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

type bearerTokenKey struct{}

var (
	// TokenList is the list of tokens that are accepted as valid
	TokenList = strings.Split(os.Getenv("TOKEN_LIST"), ",")
)

// BearerToken is a middleware that adds the bearer token included in a request's headers to context
func BearerToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		bearer := r.Header.Get("Authorization")

		if len(bearer) > 7 && strings.ToUpper(bearer[0:6]) == "BEARER" {
			token = bearer[7:]
		}
		ctx := context.WithValue(r.Context(), bearerTokenKey{}, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isSimpleTokenValid(list []string, token string) bool {
	if token == "" {
		return false
	}
	for _, validToken := range list {
		// NOTE token length information is leaked even with subtle.ConstantTimeCompare
		if subtle.ConstantTimeCompare([]byte(validToken), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

func isSimpleTokenInContext(ctx context.Context) bool {
	token, ok := ctx.Value(bearerTokenKey{}).(string)
	if !ok || !isSimpleTokenValid(TokenList, token) {
		return false
	}
	return true
}

// SimpleTokenAuthorizedOnly is a middleware that restricts access to requests with a valid bearer token via context
// NOTE the valid token is populated via BearerToken
func SimpleTokenAuthorizedOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSimpleTokenInContext(r.Context()) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
