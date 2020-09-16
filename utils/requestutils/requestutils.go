package requestutils

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/brave-intl/bat-go/utils/closers"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

type requestID string

var (
	payloadLimit10MB = int64(1024 * 1024 * 10)
	// RequestIDHeaderKey is the request header key
	RequestIDHeaderKey = "x-request-id"
	// RequestID holds the type for request ids
	RequestID = requestID(RequestIDHeaderKey)
)

// ReadWithLimit reads an io reader with a limit and closes
func ReadWithLimit(body io.Reader, limit int64) ([]byte, error) {
	defer closers.Panic(body.(io.Closer))
	return ioutil.ReadAll(io.LimitReader(body, limit))
}

// Read an io reader
func Read(body io.Reader) ([]byte, error) {
	jsonString, err := ReadWithLimit(body, payloadLimit10MB)
	if err != nil {
		return nil, errorutils.Wrap(err, "error reading body")
	}
	return jsonString, nil
}

// ReadJSON reads a request body according to an interface and limits the size to 10MB
func ReadJSON(body io.Reader, intr interface{}) error {
	jsonString, err := Read(body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonString, &intr)
	if err != nil {
		return errorutils.Wrap(err, "error unmarshalling body")
	}
	return nil
}

// SetRequestID transfers a request id from a context to a request header
func SetRequestID(ctx context.Context, r *http.Request) {
	id := GetRequestID(ctx)
	if id != "" {
		r.Header.Set(RequestIDHeaderKey, id)
	}
}

// GetRequestID gets the request id
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(RequestID).(string); ok {
		return reqID
	}
	return ""
}
