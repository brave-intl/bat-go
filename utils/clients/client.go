package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/closers"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// regular expression mapped to the replacement
var redactHeaders = map[*regexp.Regexp][]byte{
	regexp.MustCompile(`(?i)authorization: .+\n`):   []byte("Authorization: Basic <token>\n"),
	regexp.MustCompile(`(?i)x-gemini-apikey: .+\n`): []byte("X-GEMINI-APIKEY: <key>\n"),
}

func redactSensitiveHeaders(corpus []byte) []byte {
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

// New returns a new SimpleHTTPClient, retrieving the base URL from the environment
func New(serverURL string, authToken string) (*SimpleHTTPClient, error) {
	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	// with a 10 second timeout
	client := &http.Client{
		Timeout:   time.Second * 10,
		Transport: http.DefaultTransport,
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

	client := &http.Client{
		Timeout: time.Second * 10,
		Transport: middleware.InstrumentRoundTripper(
			&http.Transport{
				Proxy: proxy,
			}, name),
	}

	return &SimpleHTTPClient{
		BaseURL:   baseURL,
		AuthToken: authToken,
		client:    client,
	}, nil
}

func (c *SimpleHTTPClient) request(
	ctx context.Context,
	method string,
	resolvedURL string,
	buf io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, resolvedURL, buf)
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

	// m, _ := json.MarshalIndent(body, "", "  ")
	// fmt.Println(path, string(m))
	if body != nil && method != "GET" {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, 0, errors.Wrap(err, ErrUnableToEncodeBody)
		}
	}

	req, err := c.request(ctx, method, resolvedURL.String(), buf)
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
func (c *SimpleHTTPClient) do(
	ctx context.Context,
	req *http.Request,
	v interface{},
) (*http.Response, error) {

	tracer, propagators, err := appctx.GetOTELTracerPropagatorsFromContext(ctx, "http-client-do")
	if err != nil {
		// unexpected error
		return nil, fmt.Errorf("unexpected error drawing tracer from context: %w", err)
	}

	// get the trace id from the context
	traceID, err := appctx.GetStringFromContext(ctx, appctx.TraceIDCTXKey)
	if err != nil {
		if err == appctx.ErrNotInContext || err == appctx.ErrValueWrongType {
			// not in context, or value not a string
			// create a new traceID
			traceID = uuid.NewV4().String()
		} else {
			// unexpected error
			return nil, fmt.Errorf("unexpected error drawing trace id from context: %w", err)
		}
	}

	// make some baggage with the trace id
	traceIDMem, err := baggage.NewMember("traceID", traceID)
	if err != nil {
		// unexpected error
		return nil, fmt.Errorf("unexpected error attempting to create traceid member: %w", err)
	}
	bag, err := baggage.New(traceIDMem)
	if err != nil {
		// unexpected error
		return nil, fmt.Errorf("unexpected error creating otel baggage: %w", err)
	}

	ctx = baggage.ContextWithBaggage(ctx, bag)

	// how to get the trace id in a handler
	//ctx = propagators.Extract(ctx, propagation.HeaderCarrier(r.Header))

	var span trace.Span
	ctx, span = tracer.Start(ctx, "perform client http call span")
	defer span.End()

	// results
	var (
		resp   *http.Response
		status int
	)

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

	// dump out the full request, right before we submit it
	requestDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		panic(err)
	}
	logger.Debug().Str("type", "http.Request").Msg(string(redactSensitiveHeaders(requestDump)))

	// inject our headers from otel
	req = req.WithContext(ctx)
	// load up the request headers with otel information
	propagators.Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err = c.client.Do(req)
	if err != nil {
		return resp, err
	}
	status = resp.StatusCode
	defer closers.Panic(resp.Body)
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		panic(err)
	}

	logger.Debug().Str("type", "http.Response").Msg(string(dump))

	// // helpful if you want to read the body as it is
	// bodyBytes, _ := requestutils.Read(resp.Body)
	// resp.Body.Close() // must close
	// fmt.Println(req.URL.Host, req.URL.Path, string(bodyBytes))
	// resp.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	if status >= 200 && status <= 299 {
		if v != nil {
			err = json.NewDecoder(resp.Body).Decode(v)
			if err != nil {
				return resp, errors.Wrap(err, ErrUnableToDecode)
			}
		}
		return resp, nil
	}

	return resp, errors.Wrap(err, ErrProtocolError)
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) Do(ctx context.Context, req *http.Request, v interface{}) (*http.Response, error) {
	var (
		code      int
		header    http.Header
		resp, err = c.do(ctx, req, v)
	)
	if resp != nil {
		// it is possible to have a nil resp from c.do...
		code = resp.StatusCode
		header = resp.Header
	}
	if err != nil {
		return resp, NewHTTPError(err, req.URL.String(), "response", code, struct {
			Body    interface{}
			Headers interface{}
		}{
			Body:    v,
			Headers: req.Header,
		})
	}
	logOut(ctx, "response", *req.URL, code, header, v)
	return resp, nil
}

func logOut(
	ctx context.Context,
	outType string,
	url url.URL,
	status int,
	headers http.Header,
	body interface{},
) {

	logger := log.Ctx(ctx)
	hash := map[string]interface{}{
		"url":     url.String(),
		"body":    body,
		"headers": headers,
	}
	if status != 0 {
		hash["status"] = status
	}
	input, err := json.Marshal(hash)
	if err != nil {
		sentry.CaptureException(err)
	} else {
		logger.Debug().
			Str("type", "http."+outType).
			RawJSON(outType, redactSensitiveHeaders(input)).
			Msg(outType + " dump")
	}
}
