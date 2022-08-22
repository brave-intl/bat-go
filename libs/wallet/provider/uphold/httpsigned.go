package uphold

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"golang.org/x/net/http/httpguts"
)

// HTTPSignedRequest encapsulates a signed HTTP request
type HTTPSignedRequest struct {
	Headers map[string]string `json:"headers" valid:"-"`
	Body    string            `json:"octets" valid:"json"`
}

// extract from the encapsulated signed request
// into the provided HTTP request
// NOTE it intentionally does not set the URL
func (sr *HTTPSignedRequest) extract(r *http.Request) (*httpsignature.SignatureParams, error) {
	if r == nil {
		return nil, errors.New("r was nil")
	}

	r.Body = ioutil.NopCloser(bytes.NewBufferString(sr.Body))
	if r.Header == nil {
		r.Header = http.Header{}
	}
	for k, v := range sr.Headers {
		if !httpguts.ValidHeaderFieldName(k) {
			return nil, errors.New("invalid encapsulated header name")
		}
		if !httpguts.ValidHeaderFieldValue(v) {
			return nil, errors.New("invalid encapsulated header value")
		}

		if k == httpsignature.RequestTargetHeader {
			// TODO implement pseudo-header
			return nil, fmt.Errorf("%s pseudo-header not implemented", httpsignature.RequestTargetHeader)
		}

		r.Header.Set(k, v)
	}

	return httpsignature.SignatureParamsFromRequest(r)
}

// encapsulate a signed HTTP request
func encapsulate(req *http.Request) (*HTTPSignedRequest, error) {
	s, err := httpsignature.SignatureParamsFromRequest(req)
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

	bodyBytes, err := requestutils.Read(req.Context(), req.Body)
	if err != nil {
		return nil, err
	}
	enc.Body = string(bodyBytes)
	return &enc, nil
}
