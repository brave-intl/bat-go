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

// httpSignedRequest encapsulates a signed HTTP request
type httpSignedRequest struct {
	Headers map[string]string `json:"headers" valid:"-"`
	Body    string            `json:"octets" valid:"json"`
}

// extract an HTTP request from the encapsulated signed request
func (sr *httpSignedRequest) extract() (*httpsignature.Signature, *http.Request, error) {
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(sr.Headers["signature"]))
	if err != nil {
		return nil, nil, err
	}

	var r http.Request
	r.Body = ioutil.NopCloser(bytes.NewBufferString(sr.Body))
	r.Header = http.Header{}
	for k, v := range sr.Headers {
		if !httplex.ValidHeaderFieldName(k) {
			return nil, nil, errors.New("invalid encapsulated header name")
		}
		if !httplex.ValidHeaderFieldValue(v) {
			return nil, nil, errors.New("invalid encapsulated header value")
		}

		if k == httpsignature.RequestTarget {
			// TODO implement pseudo-header
			return nil, nil, fmt.Errorf("%s pseudo-header not implemented", httpsignature.RequestTarget)
		}

		r.Header.Set(k, v)
	}
	return &s, &r, nil
}

// encapsulate a signed HTTP request
func encapsulate(req *http.Request) (*httpSignedRequest, error) {
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(req.Header.Get("signature")))
	if err != nil {
		return nil, err
	}

	enc := httpSignedRequest{}
	enc.Headers = make(map[string]string)
	for _, k := range s.Headers {
		values := req.Header[http.CanonicalHeaderKey(k)]
		enc.Headers[k] = strings.Join(values, ", ")
	}
	enc.Headers["signature"] = req.Header.Get("signature")

	// TODO implement pseudo-header

	bodyBytes, _ := ioutil.ReadAll(req.Body)
	enc.Body = string(bodyBytes)
	return &enc, nil
}
