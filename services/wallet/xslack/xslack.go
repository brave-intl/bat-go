package xslack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	doer  httpDoer
	swURL string
}

func NewClient(httpDoer httpDoer, swURL string) *Client {
	return &Client{
		doer:  httpDoer,
		swURL: swURL,
	}
}

type Message struct {
	Channel  string  `json:"channel"`
	Username string  `json:"username"`
	Blocks   []Block `json:"blocks"`
}

type Block struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

type Text struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

func (c *Client) SendMessage(ctx context.Context, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.swURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doer.Do(req)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xslack: failed to send message: status code %d", resp.StatusCode)
	}

	return nil
}
