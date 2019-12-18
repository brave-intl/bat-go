package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/rs/zerolog/log"
)

// SimpleHTTPClient wraps http.Client for making simple token authorized requests
type SimpleHTTPClient struct {
	BaseURL   *url.URL
	AuthToken string

	client *http.Client
}

// New returns a new SimpleHTTPClient, retrieving the base URL from the environment
func New(serverEnvKey string, tokenEnvKey string) (*SimpleHTTPClient, error) {
	serverURL := os.Getenv(serverEnvKey)

	if len(serverURL) == 0 {
		return nil, errors.New(serverEnvKey + " was empty")
	}

	baseURL, err := url.Parse(serverURL)

	if err != nil {
		return nil, err
	}

	return &SimpleHTTPClient{
		BaseURL:   baseURL,
		AuthToken: os.Getenv(tokenEnvKey),
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}, nil
}

// NewRequest creaates a request, JSON encoding the body passed
func (c *SimpleHTTPClient) NewRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	resolvedURL := c.BaseURL.ResolveReference(&url.URL{Path: path})

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(body)
		if err != nil {
			return nil, handlers.AppError{
				Cause:   err,
				Message: "request",
			}
		}
	}

	req, err := http.NewRequest(method, resolvedURL.String(), buf)
	if err != nil {
		status := 0
		message := ""
		switch err.(type) {
		case url.EscapeError:
			status = http.StatusBadRequest
			message = ": unable to escape url"
		case url.InvalidHostError:
			status = http.StatusBadRequest
			message = ": invalid host"
		}
		return nil, handlers.AppError{
			Cause:   err,
			Code:    status,
			Message: fmt.Sprintf("request%s", message),
		}
	}
	req.Header.Set("accept", "application/json")

	if body != nil {
		req.Header.Add("content-type", "application/json")
	}

	logger := log.Ctx(ctx)

	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		panic(err)
	}

	logger.Debug().Str("type", "http.Request").Msg(string(dump))

	req.Header.Set("authorization", "Bearer "+c.AuthToken)

	return req, nil
}

// Do the specified http request, decoding the JSON result into v
func (c *SimpleHTTPClient) Do(ctx context.Context, req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.client.Do(req)
	status := resp.StatusCode
	if err != nil {
		return nil, handlers.AppError{
			Message: "response",
			Code:    status,
			Cause:   err,
		}
	}
	defer closers.Panic(resp.Body)
	logger := log.Ctx(ctx)
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		panic(err)
	}
	logger.Debug().Str("type", "http.Response").Msg(string(dump))

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if v != nil {
			err = json.NewDecoder(resp.Body).Decode(v)
			if err != nil {
				data := v.(interface{})
				return resp, handlers.AppError{
					Message: "response",
					Code:    status,
					Data:    data,
					Cause:   err,
				}
			}
		}
		return resp, nil
	}
	return resp, handlers.AppError{
		Message: "response",
		Code:    status,
		Cause:   fmt.Errorf("Request error: %d", status),
	}
}
