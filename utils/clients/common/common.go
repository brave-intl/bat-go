package common

import (
	"github.com/brave-intl/bat-go/utils/clients/ratios"
)

// Config holds the configuration for generating a client
type Config struct {
	Ratios bool
}

// Clients holds all of the clients
type Clients struct {
	Ratios ratios.Client
}

// New creates a new common space for clients
func New(config Config) (*Clients, error) {
	clients := Clients{}
	if config.Ratios {
		rClient, err := ratios.New()
		if err != nil {
			return nil, err
		}
		clients.Ratios = rClient
	}
	return &clients, nil
}
