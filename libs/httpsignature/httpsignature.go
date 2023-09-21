// Package httpsignature contains methods for signing and verifing HTTP requests per
// https://www.ietf.org/id/draft-cavage-http-signatures-08.txt
package httpsignature

import (
	"bytes"
	"context"
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

	"github.com/brave-intl/bat-go/libs/digest"
	"github.com/brave-intl/bat-go/libs/requestutils"
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
// NOTE that this is a subset of the crypto.Signer interface
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

// ParameterizedSignatorResponseWriter wraps a response writer to sign the response automatically
type ParameterizedSignatorResponseWriter struct {
	ParameterizedSignator
	w          http.ResponseWriter
	statusCode int
}

// Keystore provides a way to lookup a public key based on the keyID a request was signed with
type Keystore interface {
	// LookupVerifier based on the keyID
	LookupVerifier(ctx context.Context, keyID string) (context.Context, *Verifier, error)
}

// StaticKeystore is a keystore that always returns a static verifier independent of keyID
type StaticKeystore struct {
	Verifier
}

// ParameterizedKeystoreVerifier is an interface for cryptographic signature verification
type ParameterizedKeystoreVerifier struct {
	SignatureParams
	Keystore Keystore
	Opts     crypto.SignerOpts
}

const (
	// HostHeader is the host header
	HostHeader = "host"
	// DigestHeader is the header where a digest of the body will be stored
	DigestHeader = "digest"
	// RequestTargetHeader is a pseudo header consisting of the HTTP method and request uri
	RequestTargetHeader = "(request-target)"
)

var (
	signatureRegex = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// LookupVerifier by returning a static verifier
func (sk *StaticKeystore) LookupVerifier(ctx context.Context, keyID string) (context.Context, *Verifier, error) {
	return ctx, &sk.Verifier, nil
}

// TODO Add New function
// NOTE New function should check that all added headers are lower-cased

// IsMalformed returns true if the signature parameters are invalid
func (sp *SignatureParams) IsMalformed() bool {
	if sp.Algorithm == invalid {
		return true
	}
	for _, header := range sp.Headers {
		if header != strings.ToLower(header) {
			return true // all headers must be lower-cased
		}
	}
	return false
}

// BuildSigningString builds the signing string according to the SignatureParams s and
// HTTP request req
func (sp *SignatureParams) BuildSigningString(req *http.Request) (out []byte, err error) {
	if req.Body != nil {
		body, err := requestutils.Read(context.Background(), req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		return sp.buildSigningString(body, req.Header, req)
	}
	return sp.buildSigningString(nil, req.Header, req)
}

// BuildSigningStringForResponse builds the signing string according to the SignatureParams s and
// HTTP response resp
func (sp *SignatureParams) BuildSigningStringForResponse(resp *http.Response) (out []byte, err error) {
	if resp.Body != nil {
		body, err := requestutils.Read(context.Background(), resp.Body)
		if err != nil {
			return out, err
		}
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		return sp.buildSigningString(body, resp.Header, resp.Request)
	}
	return sp.buildSigningString(nil, resp.Header, resp.Request)
}

func (sp *SignatureParams) buildSigningString(body []byte, headers http.Header, req *http.Request) (out []byte, err error) {
	if sp.IsMalformed() {
		return nil, errors.New("refusing to build signing string with malformed params")
	}

	signedHeaders := sp.Headers
	if len(signedHeaders) == 0 {
		signedHeaders = []string{"date"}
	}

	for i, header := range signedHeaders {
		if header == RequestTargetHeader {
			if req == nil {
				return nil, fmt.Errorf("request must be present to use the %s pseudo-header", RequestTargetHeader)
			}
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
			if sp.DigestAlgorithm != nil {
				d.Hash = *sp.DigestAlgorithm
			}

			if body != nil {
				d.Update(body)
			}
			headers.Add("Digest", d.String())
			out = append(out, []byte(fmt.Sprintf("%s: %s", "digest", d.String()))...)
		} else if header == HostHeader {
			if req == nil {
				return nil, fmt.Errorf("request must be present to use the Host header")
			}
			// in some environments it seems that the HostTransfer middleware correctly sets
			// the Host header to the xforwardedhost value
			host := headers.Get(requestutils.HostHeaderKey)
			if host == "" {
				host = req.Host
			} else {
				host = strings.Join(headers[http.CanonicalHeaderKey(header)], ", ")
			}
			out = append(out, []byte(fmt.Sprintf("%s: %s", "host", host))...)
		} else {
			val := strings.Join(headers[http.CanonicalHeaderKey(header)], ", ")
			out = append(out, []byte(fmt.Sprintf("%s: %s", header, val))...)
		}

		if i != len(sp.Headers)-1 {
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
	return sp.sign(signator, opts, ss, req.Header)
}

// SignResponse using signator and options opts
func (sp *SignatureParams) SignResponse(signator Signator, opts crypto.SignerOpts, resp *http.Response) error {
	ss, err := sp.BuildSigningStringForResponse(resp)
	if err != nil {
		return err
	}
	return sp.sign(signator, opts, ss, resp.Header)
}

func (sp *SignatureParams) sign(signator Signator, opts crypto.SignerOpts, ss []byte, headers http.Header) error {
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
	headers.Set("Signature", string(sHeader))
	return nil
}

// SignRequest using signator and options opts in the parameterized signator
func (p *ParameterizedSignator) SignRequest(req *http.Request) error {
	return p.SignatureParams.Sign(p.Signator, p.Opts, req)
}

// SignResponse using signator and options opts in the parameterized signator
func (p *ParameterizedSignator) SignResponse(resp *http.Response) error {
	return p.SignatureParams.SignResponse(p.Signator, p.Opts, resp)
}

// NewParameterizedSignatorResponseWriter wraps the provided response writer and signs the response
func NewParameterizedSignatorResponseWriter(p ParameterizedSignator, w http.ResponseWriter) *ParameterizedSignatorResponseWriter {
	return &ParameterizedSignatorResponseWriter{
		ParameterizedSignator: p,
		w:          w,
		statusCode: -1,
	}
}

// Header returns the header map that will be sent by
// WriteHeader.
func (psrw ParameterizedSignatorResponseWriter) Header() http.Header {
	return psrw.w.Header()
}

// WriteHeader sends an HTTP response header with the provided
// status code.
func (psrw ParameterizedSignatorResponseWriter) WriteHeader(statusCode int) {
	psrw.statusCode = statusCode
}

// Write writes the data to the connection as part of an HTTP reply.
//
// For the ResponseWriter, we also sign the response before writing it out
func (psrw ParameterizedSignatorResponseWriter) Write(body []byte) (int, error) {
	ss, err := psrw.SignatureParams.buildSigningString(body, psrw.Header(), nil)
	if err != nil {
		return -1, err
	}
	err = psrw.SignatureParams.sign(psrw.Signator, psrw.Opts, ss, psrw.Header())
	if err != nil {
		return -1, err
	}

	psrw.w.WriteHeader(psrw.statusCode)
	return psrw.w.Write(body)
}

// Verify the HTTP signature s over HTTP request req using verifier with options opts
func (sp *SignatureParams) Verify(verifier Verifier, opts crypto.SignerOpts, req *http.Request) (bool, error) {
	signingStr, err := sp.BuildSigningString(req)
	if err != nil {
		return false, err
	}
	return sp.verify(verifier, opts, signingStr, req.Header)
}

// VerifyResponse by verify the HTTP signature over HTTP response resp using verifier with options opts
func (sp *SignatureParams) VerifyResponse(verifier Verifier, opts crypto.SignerOpts, resp *http.Response) (bool, error) {
	signingStr, err := sp.BuildSigningStringForResponse(resp)
	if err != nil {
		return false, err
	}
	return sp.verify(verifier, opts, signingStr, resp.Header)
}

func (sp *SignatureParams) verify(verifier Verifier, opts crypto.SignerOpts, ss []byte, headers http.Header) (bool, error) {
	var tmp signature
	err := tmp.UnmarshalText([]byte(headers.Get("Signature")))
	if err != nil {
		return false, err
	}

	sig, err := base64.StdEncoding.DecodeString(tmp.Sig)
	if err != nil {
		return false, err
	}
	return verifier.Verify(ss, sig, opts)
}

// VerifyRequest using keystore to lookup verifier with options opts
// returns the key id if the signature is valid and an error otherwise
func (pkv *ParameterizedKeystoreVerifier) VerifyRequest(req *http.Request) (context.Context, string, error) {
	sp, err := SignatureParamsFromRequest(req)
	if err != nil {
		return nil, "", err
	}

	ctx, verifier, err := pkv.Keystore.LookupVerifier(req.Context(), sp.KeyID)
	if err != nil {
		return nil, "", err
	}

	if verifier == nil {
		return nil, "", fmt.Errorf("no verifier matching keyId %s was found", sp.KeyID)
	}

	// Override algorithm and headers to those we want to enforce
	sp.Algorithm = pkv.SignatureParams.Algorithm
	sp.Headers = pkv.SignatureParams.Headers

	valid, err := sp.Verify(*verifier, pkv.Opts, req)
	if err != nil {
		return nil, "", err
	}
	if !valid {
		return nil, "", errors.New("signature is not valid")
	}

	return ctx, sp.KeyID, nil
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
	if len(text) == 0 {
		return errors.New("signature header is empty")
	}

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

// SignatureParamsFromRequest extracts the signature parameters from a signed http request
func SignatureParamsFromRequest(req *http.Request) (*SignatureParams, error) {
	var s signature
	err := s.UnmarshalText([]byte(req.Header.Get("Signature")))
	if err != nil {
		return nil, err
	}
	return &s.SignatureParams, nil
}
