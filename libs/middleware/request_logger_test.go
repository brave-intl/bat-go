package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-chi/chi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestRequestLogger_LogsPanic(t *testing.T) {
	buffer := &bytes.Buffer{}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: buffer}).
		With().
		Timestamp().
		Logger()

	ctx := logger.WithContext(context.Background())

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("panicky handler")
	})

	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil).
		WithContext(ctx)

	requestLoggerMiddleware := RequestLogger(nil)

	router := chi.NewRouter()
	router.Handle("/", requestLoggerMiddleware(panicHandler))

	server := &http.Server{Addr: ":8080", Handler: router}
	server.Handler.ServeHTTP(rw, r)

	actual := buffer.String()

	assert.Contains(t, actual, "panic recovered")
	assert.Regexp(t, regexp.MustCompile("panic=.+panicky handler"), actual)
	assert.Regexp(t, regexp.MustCompile("stacktrace=.+"), actual)
}
