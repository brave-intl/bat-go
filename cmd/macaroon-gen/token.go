package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"
)

type Caveats map[string]string

type Token struct {
	ID                string    `yaml:"id"`
	Version           int       `yaml:"version"`
	Location          string    `yaml:"location"`
	FirstPartyCaveats []Caveats `yaml:"first_party_caveats"`
}

func (t Token) Generate(secret string) (string, error) {
	// create a new macaroon
	m, err := macaroon.New([]byte(secret), []byte(t.ID), t.Location, macaroon.Version(t.Version))
	if err != nil {
		return "", fmt.Errorf("error creating new macaroon: %w", err)
	}

	for _, caveat := range t.FirstPartyCaveats {
		for k, v := range caveat {
			m.AddFirstPartyCaveat(
				[]byte(fmt.Sprintf("%s=%s", k, v)))
		}
	}

	b, err := m.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("error marshal-ing token: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

type TokenConfig struct {
	Tokens []Token `yaml:"tokens"`
}

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
