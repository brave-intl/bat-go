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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/getsentry/raven-go"
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

			entry := hlog.FromRequest(r)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				t2 := time.Now()

				// Recover and record stack traces in case of a panic
				if rec := recover(); rec != nil {
					entry.Panic().Stack()

					// Send panic info to Sentry
					recStr := fmt.Sprint(rec)
					packet := raven.NewPacket(
						recStr,
						raven.NewException(errors.New(recStr), raven.NewStacktrace(2, 3, nil)),
						raven.NewHttp(r),
					)
					raven.Capture(packet, nil)

					handlers.AppError{
						Message: http.StatusText(http.StatusInternalServerError),
						Code:    http.StatusInternalServerError,
					}.ServeHTTP(w, r)
				}

				// Log the entry, the request is complete.
				entry.Debug().
					Int("status", ww.Status()).
					Int("size", ww.BytesWritten()).
					Dur("duration", t2.Sub(t1))
			}()

			r = r.WithContext(entry.WithContext(r.Context()))
			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}
