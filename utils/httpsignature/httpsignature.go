// Package httpsignature contains methods for signing and verifing HTTP requests per
// https://www.ietf.org/id/draft-cavage-http-signatures-08.txt
package httpsignature

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/brave-intl/bat-go/utils/digest"
	"github.com/brave-intl/bat-go/utils/requestutils"
)

// SignatureParams contains parameters needed to create and verify signatures
type SignatureParams struct {
	Algorithm       Algorithm
	KeyID           string
	DigestAlgorithm *crypto.Hash // optional
	Headers         []string     // optional
}

// signature is an internal represention of an http signature and it's parameters
type signature struct {
	SignatureParams
	Sig string
}

// Signator is an interface for cryptographic signature creation
type Signator interface {
	Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error)
}

// Verifier is an interface for cryptographic signature verification
type Verifier interface {
	Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error)
	String() string
}

// ParameterizedSignator contains the parameters / options needed to create signatures and a signator
type ParameterizedSignator struct {
	SignatureParams
	Signator Signator
	Opts     crypto.SignerOpts
}

const (
	// DigestHeader is the header where a digest of the body will be stored
	DigestHeader = "digest"
	// RequestTargetHeader is a pseudo header consisting of the HTTP method and request uri
	RequestTargetHeader = "(request-target)"
)

var (
	signatureRegex = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// TODO Add New function
// NOTE New function should check that all added headers are lower-cased

// IsMalformed returns true if the signature parameters are invalid
func (s *SignatureParams) IsMalformed() bool {
	if s.Algorithm == invalid {
		return true
	}
	for _, header := range s.Headers {
		if header != strings.ToLower(header) {
			return true // all headers must be lower-cased
		}
	}
	return false
}

// BuildSigningString builds the signing string according to the SignatureParams s and
// HTTP request req
// TODO Add support for digest generation based on req.Body?
func (s *SignatureParams) BuildSigningString(req *http.Request) (out []byte, err error) {
	if s.IsMalformed() {
		return nil, errors.New("refusing to build signing string with malformed params")
	}

	headers := s.Headers
	if len(headers) == 0 {
		headers = []string{"date"}
	}

	for i, header := range headers {
		if header == RequestTargetHeader {
			if req.URL != nil && len(req.Method) > 0 {
				out = append(out, []byte(fmt.Sprintf("%s: %s %s", RequestTargetHeader, strings.ToLower(req.Method), req.URL.RequestURI()))...)
			} else {
				return nil, fmt.Errorf("request must have a URL and Method to use the %s pseudo-header", RequestTargetHeader)
			}
		} else if header == DigestHeader {
			// Just like before default to SHA256
			var d digest.Instance
			d.Hash = crypto.SHA256

			// If something else is set though use that hash instead
			if s.DigestAlgorithm != nil {
				d.Hash = *s.DigestAlgorithm
			}

			if req.Body != nil {
				body, err := requestutils.Read(req.Body)
				if err != nil {
					return out, err
				}
				req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
				d.Update(body)
			}
			req.Header.Add("Digest", d.String())
			out = append(out, []byte(fmt.Sprintf("%s: %s", "digest", d.String()))...)
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

// Sign the included HTTP request req using signator and options opts
func (sp *SignatureParams) Sign(signator Signator, opts crypto.SignerOpts, req *http.Request) error {
	ss, err := sp.BuildSigningString(req)
	if err != nil {
		return err
	}
	sig, err := signator.Sign(rand.Reader, ss, opts)
	if err != nil {
		return err
	}
	s := signature{
		SignatureParams: *sp,
		Sig:             base64.StdEncoding.EncodeToString(sig),
	}

	sHeader, err := s.MarshalText()
	if err != nil {
		return err
	}
	req.Header.Set("Signature", string(sHeader))
	return nil
}

// Sign the included HTTP request req using signator and options opts
func (p *ParameterizedSignator) SignRequest(req *http.Request) error {
	return p.SignatureParams.Sign(p.Signator, p.Opts, req)
}

// Verify the HTTP signature s over HTTP request req using verifier with options opts
func (s *SignatureParams) Verify(verifier Verifier, opts crypto.SignerOpts, req *http.Request) (bool, error) {
	signingStr, err := s.BuildSigningString(req)
	if err != nil {
		return false, err
	}

	var tmp signature
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

// MarshalText marshalls the signature into text.
func (s *signature) MarshalText() (text []byte, err error) {
	if s.IsMalformed() {
		return nil, errors.New("not a valid Algorithm")
	}

	algo, err := s.Algorithm.MarshalText()
	if err != nil {
		return nil, err
	}

	headers := ""
	if len(s.Headers) > 0 {
		headers = fmt.Sprintf(",headers=\"%s\"", strings.Join(s.Headers, " "))
	}

	text = []byte(fmt.Sprintf("keyId=\"%s\",algorithm=\"%s\"%s,signature=\"%s\"", s.KeyID, algo, headers, s.Sig))
	return text, nil
}

// UnmarshalText unmarshalls the signature from text.
func (s *signature) UnmarshalText(text []byte) (err error) {
	var key string
	var value string

	s.Algorithm = invalid
	s.KeyID = ""
	s.Sig = ""

	str := string(text)
	for _, m := range signatureRegex.FindAllStringSubmatch(str, -1) {
		key = m[1]
		value = m[2]

		if key == "keyId" {
			s.KeyID = value
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
			return errors.New("invalid key in signature")
		}
	}

	// Check that all required fields were present
	if s.Algorithm == invalid || len(s.KeyID) == 0 || len(s.Sig) == 0 {
		return errors.New("a valid signature MUST have algorithm, keyId, and signature keys")
	}

	return nil
}

func SignatureParamsFromRequest(req *http.Request) (*SignatureParams, error) {
	var s signature
	err := s.UnmarshalText([]byte(req.Header.Get("Signature")))
	if err != nil {
		return nil, err
	}
	return &s.SignatureParams, nil
}
