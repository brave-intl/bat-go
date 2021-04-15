package common

import (
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/ratios"
)

// Config holds the configuration for generating a client
type Config struct {
	Ratios bool
}

// Clients holds all of the clients
type Clients struct {
	ratios ratios.Client
}

// New creates a new common space for clients
func New(params ...func(*Clients) error) (*Clients, error) {
	clients := Clients{}
	for _, param := range params {
		err := param(&clients)
		if err != nil {
			return nil, err
		}
	}
	return &clients, nil
}

// WithRatios creates a ratios client on the common clients
func WithRatios(clients *Clients) error {
	rClient, err := ratios.New()
	if err == nil {
		clients.ratios = rClient
	}
	return err
}

// Ratios gets or creates the ratios client
// panics if an error is encountered during instantiation
func (clients *Clients) Ratios() ratios.Client {
	if clients.ratios == nil {
		err := WithRatios(clients)
		if err != nil {
			panic(fmt.Errorf("unable to setup clients with ratios, try setting up before using %v", err))
		}
	}
	return clients.ratios
}
