package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"

	"github.com/brave-intl/bat-go/libs/closers"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// regular expression mapped to the replacement
var redactHeaders = map[*regexp.Regexp][]byte{
	regexp.MustCompile(`(?i)authorization: (?i)basic.+\n`):  []byte("Authorization: Basic <token>\n"),
	regexp.MustCompile(`(?i)authorization: (?i)bearer.+\n`): []byte("Authorization: Bearer <token>\n"),
	regexp.MustCompile(`(?i)x-gemini-apikey: .+\n`):         []byte("X-GEMINI-APIKEY: <key>\n"),
	regexp.MustCompile(`(?i)signature: .+\n`):               []byte("Signature: <sig>\n"),
	regexp.MustCompile(`(?i)x_cg_pro_api_key=.+\n`):         []byte("x_cg_pro_api_key:<key>\n"),
	regexp.MustCompile(`(?i)x_cg_pro_api_key=.+&`):          []byte("x_cg_pro_api_key:<key>&"),
}

// RedactSensitiveHeaders from http request dumps
func RedactSensitiveHeaders(corpus []byte) []byte {
	for k, v := range redactHeaders {
		corpus = k.ReplaceAll(corpus, v)
	}
	return corpus
}

var concurrentClientRequests = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "concurrent_client_requests",
		Help: "Gauge that holds the current number of client requests",
	},
	[]string{
		"host",
		"method",
	},
)

func init() {
	prometheus.MustRegister(concurrentClientRequests)
}

// QueryStringBody - a type to generate the query string from a request "body" for the client
type QueryStringBody interface {
	// GenerateQueryString - function to generate the query string
	GenerateQueryString() (url.Values, error)
}

// SimpleHTTPClient wraps http.Client for making simple token authorized requests
type SimpleHTTPClient struct {
	BaseURL   *url.URL
	AuthToken string

	client *http.Client
}

// New returns a new SimpleHTTPClient
func New(serverURL string, authToken string) (*SimpleHTTPClient, error) {
	return NewWithHTTPClient(serverURL, authToken, &http.Client{
		Timeout: time.Second * 10,
	})
}

// NewWithHTTPClient returns a new SimpleHTTPClient, using the provided http.Client
func NewWithHTTPClient(serverURL string, authToken string, client *http.Client) (*SimpleHTTPClient, error) {
	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	return &SimpleHTTPClient{
		BaseURL:   baseURL,
		AuthToken: authToken,
		client:    client,
	}, nil
}

// NewWithProxy returns a new SimpleHTTPClient, retrieving the base URL from the environment and adds a proxy
func NewWithProxy(name string, serverURL string, authToken string, proxyURL string) (*SimpleHTTPClient, error) {
	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	var proxy func(*http.Request) (*url.URL, error)
	if len(proxyURL) != 0 {
		proxiedURL, err := url.Parse(proxyURL)
		if err != nil {
			panic("HTTP_PROXY is not a valid proxy URL")
		}
		proxy = http.ProxyURL(proxiedURL)
	} else {
		proxy = nil
	}
	fmt.Printf("BASEURL: %v\n", baseURL)
	return &SimpleHTTPClient{
		BaseURL:   baseURL,
		AuthToken: authToken,
		client: &http.Client{
			Timeout: time.Second * 10,
			Transport: middleware.InstrumentRoundTripper(
				&http.Transport{
					Proxy: proxy,
				}, name),
		},
	}, nil
}

func (c *SimpleHTTPClient) request(
	method string,
	resolvedURL string,
	buf io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequest(method, resolvedURL, buf)
	if err != nil {
		switch err.(type) {
		case url.EscapeError:
			err = NewHTTPError(err, resolvedURL, ErrUnableToEscapeURL, http.StatusBadRequest, nil)
		case url.InvalidHostError:
			err = NewHTTPError(err, resolvedURL, ErrInvalidHost, http.StatusBadRequest, nil)
		default:
			err = NewHTTPError(err, resolvedURL, ErrMalformedRequest, http.StatusBadRequest, nil)
		}
		return nil, err
	}
	return req, nil
}

// newRequest creaates a request, JSON encoding the body passed
func (c *SimpleHTTPClient) newRequest(
	ctx context.Context,
	method,
	path string,
	body interface{},
	qsb QueryStringBody,
) (*http.Request, int, error) {
	var buf io.ReadWriter
	qs := ""

	if qsb != nil {
		v, err := qsb.GenerateQueryString()
		if err != nil {
			// problem generating the query string from the type
			return nil, 0, fmt.Errorf("failed to generate query string: %w", err)
		}
		qs = v.Encode()
	}

	resolvedURL := c.BaseURL.ResolveReference(&url.URL{
		Path:     path,
		RawQuery: qs,
	})

	if body != nil && method != "GET" {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, 0, errors.Wrap(err, ErrUnableToEncodeBody)
		}
	}

	req, err := c.request(method, resolvedURL.String(), buf)
	if err != nil {
		status := 0
		switch err.(type) {
		case url.EscapeError:
			status = http.StatusBadRequest
			err = errors.Wrap(err, ErrUnableToEscapeURL)
		case url.InvalidHostError:
			status = http.StatusBadRequest
			err = errors.Wrap(err, ErrInvalidHost)
		}
		return nil, status, err
	}

	req.Header.Set("accept", "application/json")
	if body != nil {
		req.Header.Add("content-type", "application/json")
	}
	requestutils.SetRequestID(ctx, req)
	if c.AuthToken != "" {
		req.Header.Set("authorization", "Bearer "+c.AuthToken)
	}
	return req, 0, nil
}

// NewRequest wraps the new request with a particular error type
func (c *SimpleHTTPClient) NewRequest(
	ctx context.Context,
	method,
	path string,
	body interface{},
	qsb QueryStringBody,
) (*http.Request, error) {
	req, status, err := c.newRequest(ctx, method, path, body, qsb)
	if err != nil {
		return nil, NewHTTPError(err, (*req.URL).String(), "request", status, body)
	}
	return req, err
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) do(ctx context.Context, req *http.Request, v interface{}) (*http.Response, error) {

	// concurrent client request instrumentation
	concurrentClientRequests.With(
		prometheus.Labels{
			"host": req.URL.Host, "method": req.Method,
		}).Inc()

	defer func() {
		concurrentClientRequests.With(
			prometheus.Labels{
				"host": req.URL.Host, "method": req.Method,
			}).Dec()
	}()

	logger := log.Ctx(ctx)
	debug, okDebug := ctx.Value(appctx.DebugLoggingCTXKey).(bool)

	if okDebug && debug {
		// if debug is set, then dump response
		// dump out the full request, right before we submit it
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			logger.Error().Err(err).Str("type", "http.Request").Msg("failed to dump request body")
		} else {
			logger.Debug().Str("type", "http.Request").Msg(string(RedactSensitiveHeaders(requestDump)))
		}
	}

	// put a timeout on the request context
	reqCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	scopedCtx := appctx.Wrap(req.Context(), reqCtx)
	// cancel the context when complete
	defer cancel()

	req = req.WithContext(scopedCtx)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	status := resp.StatusCode
	defer closers.Panic(ctx, resp.Body)

	if okDebug && debug {
		// if debug is set, then dump response
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			logger.Error().Err(err).Str("type", "http.Response").Msg("failed to dump response body")
		} else {
			logger.Debug().Str("type", "http.Response").Msg(string(dump))
		}
	}

	// helpful if you want to read the body as it is
	bodyBytes, _ := requestutils.Read(ctx, resp.Body)
	_ = resp.Body.Close() // must close
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	if status >= 200 && status <= 299 {
		if v != nil {
			err = json.Unmarshal(bodyBytes, v)
			if err != nil {
				return resp, errors.Wrap(err, ErrUnableToDecode)
			}
		}

		return resp, nil
	}

	logger.Warn().
		Int("response_status", status).
		Err(err).
		Str("body", string(bodyBytes)). // add errored body into the messaging
		Msg("failed http client call")
	logger.Debug().Str("host", req.URL.Host).Str("path", req.URL.Path).Str("body", string(bodyBytes)).Msg("failed http client call")
	return resp, errors.Wrap(err, ErrProtocolError)
}

// RespErrData - error data for http response
type RespErrData struct {
	ResponseHeaders interface{}
	Body            interface{}
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) Do(ctx context.Context, req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.do(ctx, req, v)
	if err != nil {
		// errors returned from c.do could be go errors or upstream api errors
		if resp != nil {
			// if there was an error from the service, read the response body
			// and inject into error for later
			b, _ := ioutil.ReadAll(resp.Body)
			rb := string(b)
			resp.Body = ioutil.NopCloser(bytes.NewBuffer(b))

			// put response body/headers in the err state data
			errorData := RespErrData{
				ResponseHeaders: resp.Header,
				Body:            rb,
			}

			return resp, NewHTTPError(err, req.URL.String(), "response", resp.StatusCode, errorData)
		}
		return nil, fmt.Errorf("failed c.do, no response body: %w", err)
	}
	return resp, nil
}
