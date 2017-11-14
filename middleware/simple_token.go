package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

var tokenList = strings.Split(os.Getenv("TOKEN_LIST"), ",")

func BearerToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string

		bearer := r.Header.Get("Authorization")

		if len(bearer) > 7 && strings.ToUpper(bearer[0:6]) == "BEARER" {
			token = bearer[7:]
		}
		ctx := context.WithValue(r.Context(), "bearer.token", token)
		next.ServeHTTP(w, r.WithContext(ctx))
		return
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

func SimpleTokenAuthorizedOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token, ok := ctx.Value("bearer.token").(string)
		if !ok || !isSimpleTokenValid(tokenList, token) {
			http.Error(w, http.StatusText(403), 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}
