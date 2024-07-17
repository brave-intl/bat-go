package httpsignature

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/brave-intl/bat-go/libs/requestutils"
	"golang.org/x/net/http/httpguts"
)

// HTTPSignedRequest encapsulates a signed HTTP request
type HTTPSignedRequest struct {
	Headers map[string]string `json:"headers" valid:"-"`
	Body    string            `json:"octets" valid:"json"`
}

// Extract from the encapsulated signed request
// into the provided HTTP request
func (sr *HTTPSignedRequest) Extract(r *http.Request) (*SignatureParams, error) {
	if r == nil {
		return nil, errors.New("r was nil")
	}

	r.Body = ioutil.NopCloser(bytes.NewBufferString(sr.Body))
	r.ContentLength = int64(len(sr.Body))
	if r.Header == nil {
		r.Header = http.Header{}
	}
	for k, v := range sr.Headers {
		if k == RequestTargetHeader {
			method, uri, found := strings.Cut(v, " ")
			if !found {
				return nil, fmt.Errorf("invalid encapsulated %s pseudo-header value", RequestTargetHeader)
			}
			r.Method = strings.ToUpper(method)
			pURI, err := url.ParseRequestURI(uri)
			if err != nil {
				return nil, fmt.Errorf("invalid encapsulated %s pseudo-header value: %e", RequestTargetHeader, err)
			}
			r.URL = pURI
		} else {
			if !httpguts.ValidHeaderFieldName(k) {
				return nil, errors.New("invalid encapsulated header name")
			}
			if !httpguts.ValidHeaderFieldValue(v) {
				return nil, errors.New("invalid encapsulated header value")
			}

			r.Header.Set(k, v)
		}
	}

	return SignatureParamsFromRequest(r)
}

// EncapsulateRequest a signed HTTP request
func EncapsulateRequest(req *http.Request) (*HTTPSignedRequest, error) {
	s, err := SignatureParamsFromRequest(req)
	if err != nil {
		return nil, err
	}

	enc := HTTPSignedRequest{}
	enc.Headers = make(map[string]string)
	for _, k := range s.Headers {
		if k == RequestTargetHeader {
			enc.Headers[k] = formatRequestTarget(req)
		} else {
			values := req.Header[http.CanonicalHeaderKey(k)]
			enc.Headers[k] = strings.Join(values, ", ")
		}
	}
	enc.Headers["signature"] = req.Header.Get("signature")

	bodyBytes, err := requestutils.Read(req.Context(), req.Body)
	if err != nil {
		return nil, err
	}
	enc.Body = string(bodyBytes)
	return &enc, nil
}

// HTTPSignedResponse encapsulates a signed HTTP response
type HTTPSignedResponse struct {
	StatusCode int               `json:"statusCode" valid:"-"`
	Headers    map[string]string `json:"headers" valid:"-"`
	Body       string            `json:"octets" valid:"json"`
}

// Extract from the encapsulated signed response
// into the provided HTTP response
func (sr *HTTPSignedResponse) Extract(r *http.Response) (*SignatureParams, error) {
	if r == nil {
		return nil, errors.New("r was nil")
	}

	r.StatusCode = sr.StatusCode
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

		if k == RequestTargetHeader {
			// TODO implement pseudo-header
			return nil, fmt.Errorf("%s pseudo-header not implemented", RequestTargetHeader)
		}

		r.Header.Set(k, v)
	}

	return SignatureParamsFromResponse(r)
}

// EncapsulateResponse a signed HTTP response
func EncapsulateResponse(ctx context.Context, resp *http.Response) (*HTTPSignedResponse, error) {
	s, err := SignatureParamsFromResponse(resp)
	if err != nil {
		return nil, err
	}

	enc := HTTPSignedResponse{}
	enc.Headers = make(map[string]string)
	for _, k := range s.Headers {
		values := resp.Header[http.CanonicalHeaderKey(k)]
		enc.Headers[k] = strings.Join(values, ", ")
	}
	enc.Headers["signature"] = resp.Header.Get("signature")

	// TODO implement pseudo-header

	bodyBytes, err := requestutils.Read(ctx, resp.Body)
	if err != nil {
		return nil, err
	}
	enc.Body = string(bodyBytes)
	enc.StatusCode = resp.StatusCode
	return &enc, nil
}
