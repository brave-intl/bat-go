package uphold

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/brave-intl/bat-go/utils/httpsignature"
	"golang.org/x/net/lex/httplex"
)

// httpSignedRequest encapsulates a signed HTTP request
type httpSignedRequest struct {
	Headers map[string]string `json:"headers",valid:"lowercase"`
	Body    string            `json:"octets",valid:"-"`
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
