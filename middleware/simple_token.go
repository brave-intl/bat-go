package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	boom "github.com/maikelmclauflin/go-boom"
)

type bearerTokenKey struct{}

var (
	// TokenList is the list of tokens that are accepted as valid
	TokenList = strings.Split(os.Getenv("TOKEN_LIST"), ",")
	// ScopesToEnv maps the scope key to the env that holds the list
	ScopesToEnv = map[string]string{
		"referrals":  "ALLOWED_REFERRALS_TOKENS",
		"publishers": "ALLOWED_PUBLISHERS_TOKENS",
		"global":     "TOKEN_LIST",
	}
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

func isSimpleScopedTokenInContext(ctx context.Context, scopes []string) bool {
	for _, scope := range scopes {
		value := os.Getenv(ScopesToEnv[scope])
		if len(value) == 0 {
			continue
		}
		tokenList := strings.Split(value, ",")
		scopedTokens := []string{}
		for _, token := range tokenList {
			scopedTokens = append(scopedTokens, strings.TrimSpace(token))
		}
		token, ok := ctx.Value(bearerTokenKey{}).(string)
		if ok && isSimpleTokenValid(scopedTokens, token) {
			return true
		}
	}
	return false
}

// SimpleScopedTokenAuthorizedOnly is a middleware that restricts access
// to requests with a valid bearer token via context
// NOTE the valid token is populated via BearerToken
// the scopes passed will check the token against multiple values
func SimpleScopedTokenAuthorizedOnly(next http.Handler, scopes ...string) http.Handler {
	if len(scopes) == 0 {
		scopes = append(scopes, "global")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSimpleScopedTokenInContext(r.Context(), scopes) {
			boom.RenderForbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
