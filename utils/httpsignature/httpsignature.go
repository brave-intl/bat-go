package httpsignature

// https://www.ietf.org/id/draft-cavage-http-signatures-08.txt

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/lex/httplex"
)

type SignatureParams struct {
	Algorithm Algorithm
	KeyId     string
	Headers   []string // optional
}

type Signature struct {
	SignatureParams
	Sig string // FIXME remove since we should always use http.Request header?
}

const (
	RequestTarget = "(request-target)"
)

var (
	signatureRegex = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// TODO Add New function
// TODO New function should check that all added headers are lower-cased

func (s *SignatureParams) IsMalformed() bool {
	if s.Algorithm == INVALID {
		return true
	}
	for _, header := range s.Headers {
		if header != strings.ToLower(header) {
			return true // all headers must be lower-cased
		}
	}
	return false
}

// TODO? Add support for digest generation bsed on req.Body?
func (s *SignatureParams) BuildSigningString(req *http.Request) (out []byte, err error) {
	if s.IsMalformed() {
		return nil, errors.New("Refusing to build signing string with malformed params")
	}

	headers := s.Headers
	if len(headers) == 0 {
		headers = []string{"date"}
	}

	for i, header := range headers {
		if header == RequestTarget {
			if req.URL != nil && len(req.Method) > 0 {
				out = append(out, []byte(fmt.Sprintf("%s: %s %s", RequestTarget, strings.ToLower(req.Method), req.URL.RequestURI()))...)
			} else {
				return nil, errors.New(fmt.Sprintf("Request must have a URL and Method to use the %s pseudo-header", RequestTarget))
			}
		} else {
			val := strings.Join(req.Header[http.CanonicalHeaderKey(header)], ", ")
			out = append(out, []byte(fmt.Sprintf("%s: %s", header, val))...)
		}

		if i != len(s.Headers)-1 {
			out = append(out, byte('\n'))
		}
	}
	return out, nil
}

func (s *Signature) Sign(signator crypto.Signer, opts crypto.SignerOpts, req *http.Request) error {
	ss, err := s.BuildSigningString(req)
	if err != nil {
		return err
	}
	sig, err := signator.Sign(rand.Reader, ss, opts)
	if err != nil {
		return err
	}
	s.Sig = base64.StdEncoding.EncodeToString(sig)

	sHeader, err := s.MarshalText()
	if err != nil {
		return err
	}
	req.Header.Set("Signature", string(sHeader))
	return nil
}

type Verifier interface {
	Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error)
}

func (s *Signature) Verify(verifier Verifier, opts crypto.SignerOpts, req *http.Request) (bool, error) {
	signingStr, err := s.BuildSigningString(req)
	if err != nil {
		return false, err
	}

	var tmp Signature
	err = tmp.UnmarshalText([]byte(req.Header.Get("Signature")))
	if err != nil {
		return false, err
	}

	sig, err := base64.StdEncoding.DecodeString(tmp.Sig)
	if err != nil {
		return false, err
	}
	return verifier.Verify(signingStr, sig, opts)
}

func (s *Signature) MarshalText() (text []byte, err error) {
	if s.IsMalformed() {
		return nil, errors.New("Not a valid Algorithm")
	}

	// FIXME just replace with sprintf?

	text = append(text, []byte("keyId=\"")...)
	text = append(text, []byte(s.KeyId)...)

	text = append(text, []byte("\",algorithm=\"")...)
	algo, err := s.Algorithm.MarshalText()
	if err != nil {
		return nil, err
	}
	text = append(text, algo...)

	if len(s.Headers) > 0 {
		text = append(text, []byte("\",headers=\"")...)
		text = append(text, []byte(strings.Join(s.Headers, " "))...)
	}

	text = append(text, []byte("\",signature=\"")...)
	text = append(text, []byte(s.Sig)...)
	text = append(text, []byte("\"")...)

	return text, nil
}

func (s *Signature) UnmarshalText(text []byte) (err error) {
	var key string
	var value string

	s.Algorithm = INVALID
	s.KeyId = ""
	s.Sig = ""

	str := string(text)
	for _, m := range signatureRegex.FindAllStringSubmatch(str, -1) {
		key = m[1]
		value = m[2]

		if key == "keyId" {
			s.KeyId = value
		} else if key == "algorithm" {
			err := s.Algorithm.UnmarshalText([]byte(value))
			if err != nil {
				return err
			}
		} else if key == "headers" {
			s.Headers = strings.Split(value, " ")
		} else if key == "signature" {
			s.Sig = value
		} else {
			return errors.New("Invalid key in signature")
		}
	}

	// Check that all required fields were present
	if s.Algorithm == INVALID || len(s.KeyId) == 0 || len(s.Sig) == 0 {
		return errors.New("A valid signature MUST have algorithm, keyId, and signature keys")
	}

	return nil
}

// An encapsulated HTTP signature request
type HttpSignedRequest struct {
	Headers map[string]string `json:"headers"` //TODO add verfier to ensure only lower-cased keys
	Body    string            `json:"octets"`
}

func (sr *HttpSignedRequest) Extract() (*Signature, *http.Request, error) {
	var s Signature
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

		if k == RequestTarget {
			// TODO
			return nil, nil, errors.New(fmt.Sprintf("%s pseudo-header not implemented", RequestTarget))
		} else {
			r.Header.Set(k, v)
		}
	}
	return &s, &r, nil
}

func Encapsulate(req *http.Request) (*HttpSignedRequest, error) {
	enc := HttpSignedRequest{}
	for k, values := range req.Header {
		enc.Headers[k] = strings.Join(values, ", ")
	}
	// FIXME
	//enc.Headers[RequestTarget] =
	bodyBytes, _ := ioutil.ReadAll(req.Body)
	enc.Body = string(bodyBytes)
	return &enc, nil
}
