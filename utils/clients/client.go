package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/utils/errors"
	requestutils "github.com/brave-intl/bat-go/utils/request"
	"github.com/getsentry/sentry-go"
	"github.com/google/go-querystring/query"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

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

	return &SimpleHTTPClient{
		BaseURL:   baseURL,
		AuthToken: authToken,
		client: &http.Client{
			Timeout: time.Second * 10,
		},
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
) (*http.Request, int, error) {
	var buf io.ReadWriter
	qs := ""
	if method == "GET" && body != nil {
		v, err := query.Values(body)
		if err != nil {
			return nil, 0, err
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
) (*http.Request, error) {
	req, status, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, NewHTTPError(err, (*req.URL).String(), "request", status, body)
	}
	logOut(ctx, "request", *req.URL, 0, req.Header, body)
	return req, err
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) do(
	ctx context.Context,
	req *http.Request,
	v interface{},
) (*http.Response, error) {

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

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	status := resp.StatusCode
	defer closers.Panic(resp.Body)
	logger := log.Ctx(ctx)
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
			RawJSON(outType, input).
			Msg(outType + " dump")
	}
}
