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

	"github.com/brave-intl/bat-go/utils/closers"
	raven "github.com/getsentry/raven-go"
	"github.com/rs/zerolog/log"
)

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

func (c *SimpleHTTPClient) newRequest(
	method string,
	resolvedURL string,
	buf io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequest(method, resolvedURL, buf)
	if err != nil {
		switch err.(type) {
		case url.EscapeError:
			err = NewHTTPError(err, ErrUnableToEscapeURL, http.StatusBadRequest, nil)
		case url.InvalidHostError:
			err = NewHTTPError(err, ErrInvalidHost, http.StatusBadRequest, nil)
		default:
			err = NewHTTPError(err, ErrMalformedRequest, http.StatusBadRequest, nil)
		}
		return nil, err
	}
	return req, nil
}

// NewRequest creaates a request, JSON encoding the body passed
func (c *SimpleHTTPClient) NewRequest(
	ctx context.Context,
	method,
	path string,
	body interface{},
) (*http.Request, error) {
	var buf io.ReadWriter
	resolvedURL := c.BaseURL.ResolveReference(&url.URL{Path: path})

	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, NewHTTPError(err, ErrUnableToEncodeBody, 0, nil)
		}
	}

	req, err := c.newRequest(method, resolvedURL.String(), buf)
	if err != nil {
		return req, err
	}

	req.Header.Set("accept", "application/json")
	if body != nil {
		req.Header.Add("content-type", "application/json")
	}

	logOut(ctx, "request", *req.URL, 0, req.Header, body)

	req.Header.Set("authorization", "Bearer "+c.AuthToken)

	return req, nil
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) do(
	ctx context.Context,
	req *http.Request,
	v interface{},
) (*http.Response, error) {
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

	if status >= 200 && status <= 299 {
		if v != nil {
			err = json.NewDecoder(resp.Body).Decode(v)
			if err != nil {
				return resp, NewHTTPError(err, ErrUnableToDecode, resp.StatusCode, v)
			}
		}
		return resp, nil
	}
	return resp, NewHTTPError(err, ErrProtocolError, resp.StatusCode, v)
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) Do(ctx context.Context, req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.do(ctx, req, v)
	if err != nil {
		return resp, err
	}
	logOut(ctx, "response", *req.URL, resp.StatusCode, resp.Header, v)
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
		raven.CaptureError(err, nil)
	} else {
		logger.Debug().
			Str("type", "http."+outType).
			RawJSON(outType, input).
			Msg(outType + " dump")
	}
}
