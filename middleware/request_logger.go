package middleware

/*
Copyright 2016-current lg authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * The names of authors or contributors may NOT be used to endorse or
promote products derived from this software without specific prior
written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

import (
	"fmt"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

// RequestLogger logs at the start and stop of incoming HTTP requests as well as recovers from panics
// Modified version of RequestLogger from github.com/rs/zerolog
// Added support for sending captured panic to Sentry
func RequestLogger(logger *zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			if r.URL.EscapedPath() == "/metrics" { // Skip logging prometheus metric scrapes
				next.ServeHTTP(w, r)
				return
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			t1 := time.Now().UTC()
			// only need to get logger from context once per request
			logger := hlog.FromRequest(r)
			createSubLog(logger, r, 0).
				Msg("request started")

			defer func() {
				t2 := time.Now().UTC()

				// Recover and record stack traces in case of a panic
				if rec := recover(); rec != nil {
					logger.Panic().Stack()

					// Send panic info to Sentry
					event := sentry.NewEvent()
					event.Message = fmt.Sprint(rec)
					sentry.CaptureEvent(event)

					handlers.AppError{
						Message: http.StatusText(http.StatusInternalServerError),
						Code:    http.StatusInternalServerError,
					}.ServeHTTP(w, r)
				}

				status := ww.Status()
				// Log the entry, the request is complete.
				createSubLog(logger, r, status).
					Int("status", status).
					Int("size", ww.BytesWritten()).
					Dur("duration", t2.Sub(t1)).
					Msg("request complete")
			}()

			r = r.WithContext(logger.WithContext(r.Context()))
			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}

func createSubLog(logger *zerolog.Logger, r *http.Request, status int) (subLog *zerolog.Event) {
	if status >= 400 && status <= 499 {
		subLog = logger.Warn()
	} else if status >= 500 {
		subLog = logger.Error()
	} else {
		subLog = logger.Info()
	}
	return subLog.
		Str("host", r.Host).
		Str("http_proto", r.Proto).
		Str("http_method", r.Method).
		Str("uri", r.URL.EscapedPath())
}
