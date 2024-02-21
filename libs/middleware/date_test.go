package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVerifyDateIsRecent(t *testing.T) {
	fn1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("done"))
	}

	handler := VerifyDateIsRecent(10*time.Minute, 10*time.Minute)(http.HandlerFunc(fn1))

	req, err := http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code, "request without required date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", "foo")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code, "request with invalid date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Add(time.Minute*60).Format(time.RFC1123))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooEarly, rr.Code, "request with timed out date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Add(time.Minute*-60).Format(time.RFC1123))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusRequestTimeout, rr.Code, "request with early date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Format(time.RFC1123))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "request with current date should succeed")
}
