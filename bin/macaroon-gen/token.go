package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/brave-intl/bat-go/payment"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"
)

// Caveats - configuration representation of key pair caveats
type Caveats map[string]string

// Token - configuration representation of token metadata attributes
type Token struct {
	ID                string    `yaml:"id"`
	Version           int       `yaml:"version"`
	PaymentBaseURL    string    `yaml:"payment_base_url"`
	Location          string    `yaml:"location"`
	FirstPartyCaveats []Caveats `yaml:"first_party_caveats"`
}

// Generate - Generate a Macaroon from the Token configuration
func (t Token) Generate(secret string) (string, error) {
	// if secret is "" try to get the secret from the merchant api
	if secret == "" {
		// fetch from merchant api
		// GET /v1/merchants/${t.Location}/keys/
		if t.PaymentBaseURL != "" {
			resp, err := http.Get(
				fmt.Sprintf("%s/v1/merchants/%s/keys/", t.PaymentBaseURL, t.Location))

			if err != nil {
				panic(fmt.Sprintf("unable to GET keys endpoint: %s", err))
			}
			// take the results and get the first key??
			var keys = []*payment.Key{}
			if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
				panic(fmt.Sprintf("unable to decode response body: %s", err))
			}
			if len(keys) > 0 {
				secret = keys[0].SecretKey
			} else {
				panic("no keys for merchant")
			}
		} else {
			panic("cannot make a macaroon without a secret")
		}
	}

	// create a new macaroon
	m, err := macaroon.New([]byte(secret), []byte(t.ID), t.Location, macaroon.Version(t.Version))
	if err != nil {
		return "", fmt.Errorf("error creating new macaroon: %w", err)
	}

	for _, caveat := range t.FirstPartyCaveats {
		for k, v := range caveat {
			err := m.AddFirstPartyCaveat(
				[]byte(fmt.Sprintf("%s=%s", k, v)))
			if err != nil {
				return "", fmt.Errorf("failed to add caveat: %w", err)
			}
		}
	}

	b, err := m.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("error marshal-ing token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// TokenConfig - structure of the token configuration file
type TokenConfig struct {
	Tokens []Token `yaml:"tokens"`
}

// Parse - Parse the token configuration file
func (tc *TokenConfig) Parse(path string) (err error) {
	// read file
	buf, err := os.Open(path)
	if err != nil {
		err = fmt.Errorf("unable to open file: %w", err)
		return
	}
	defer func() {
		// close out file correctly
		if err = buf.Close(); err != nil {
			err = fmt.Errorf("unable to close file: %w", err)
			return
		}
	}()

	// parse file
	if err = yaml.NewDecoder(buf).Decode(tc); err != nil {
		err = fmt.Errorf("unable to parse file: %w", err)
		return
	}
	return
}
