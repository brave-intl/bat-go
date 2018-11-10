package uphold

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/brave-intl/bat-go/utils/httpsignature"
	"golang.org/x/net/lex/httplex"
)

// HTTPSignedRequest encapsulates a signed HTTP request
type HTTPSignedRequest struct {
	Headers map[string]string `json:"headers" valid:"-"`
	Body    string            `json:"octets" valid:"json"`
}

// extract from the encapsulated signed request
// into the provided HTTP request
// NOTE it intentionally does not set the URL
func (sr *HTTPSignedRequest) extract(r *http.Request) (*httpsignature.Signature, error) {
	if r == nil {
		return nil, errors.New("r was nil")
	}

	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(sr.Headers["signature"]))
	if err != nil {
		return nil, err
	}

	r.Body = ioutil.NopCloser(bytes.NewBufferString(sr.Body))
	if r.Header == nil {
		r.Header = http.Header{}
	}
	for k, v := range sr.Headers {
		if !httplex.ValidHeaderFieldName(k) {
			return nil, errors.New("invalid encapsulated header name")
		}
		if !httplex.ValidHeaderFieldValue(v) {
			return nil, errors.New("invalid encapsulated header value")
		}

		if k == httpsignature.RequestTarget {
			// TODO implement pseudo-header
			return nil, fmt.Errorf("%s pseudo-header not implemented", httpsignature.RequestTarget)
		}

		r.Header.Set(k, v)
	}
	return &s, nil
}

// encapsulate a signed HTTP request
func encapsulate(req *http.Request) (*HTTPSignedRequest, error) {
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(req.Header.Get("signature")))
	if err != nil {
		return nil, err
	}

	enc := HTTPSignedRequest{}
	enc.Headers = make(map[string]string)
	for _, k := range s.Headers {
		values := req.Header[http.CanonicalHeaderKey(k)]
		enc.Headers[k] = strings.Join(values, ", ")
	}
	enc.Headers["signature"] = req.Header.Get("signature")

	// TODO implement pseudo-header

	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	enc.Body = string(bodyBytes)
	return &enc, nil
}
