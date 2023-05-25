package requestutils

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/brave-intl/bat-go/libs/closers"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
)

type requestID string

var (
	payloadLimit10MB = int64(1024 * 1024 * 10)
	// RequestIDHeaderKey is the request header key
	RequestIDHeaderKey = "x-request-id"
	// RequestID holds the type for request ids
	RequestID = requestID(RequestIDHeaderKey)
	// HostHeaderKey is the request header key
	HostHeaderKey = "host"
	// XForwardedHostHeaderKey is the request header key
	XForwardedHostHeaderKey = "x-forwarded-host"
)

// ReadWithLimit reads an io reader with a limit and closes
func ReadWithLimit(ctx context.Context, body io.Reader, limit int64) ([]byte, error) {
	defer closers.Panic(ctx, body.(io.Closer))
	return ioutil.ReadAll(io.LimitReader(body, limit))
}

// Read an io reader
func Read(ctx context.Context, body io.Reader) ([]byte, error) {
	jsonString, err := ReadWithLimit(ctx, body, payloadLimit10MB)
	if err != nil {
		return nil, errorutils.Wrap(err, "error reading body")
	}
	return jsonString, nil
}

// ReadJSON reads a request body according to an interface and limits the size to 10MB
func ReadJSON(ctx context.Context, body io.Reader, intr interface{}) error {
	logger := logging.Logger(ctx, "requestutils.ReadJSON")
	if body == nil {
		return errorutils.New(errors.New("body is nil"), "Error in request body", nil)
	}
	jsonString, err := Read(ctx, body)
	if err != nil {
		return err
	}
	logger.Debug().Str("json", string(jsonString)).Msg("read payload")
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
