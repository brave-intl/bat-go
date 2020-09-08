// Package pindialer contains a basic implementation of a pinned HTTPS dialer
package pindialer

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
)

// ContextDialer is a function connecting to the address on the named network
type ContextDialer func(ctx context.Context, network, addr string) (net.Conn, error)

func validateChain(fingerprint string, connstate tls.ConnectionState) error {
	for _, chain := range connstate.VerifiedChains {
		leafCert := chain[0]
		hash := sha256.Sum256(leafCert.RawSubjectPublicKeyInfo)
		digest := base64.StdEncoding.EncodeToString(hash[:])
		if digest == fingerprint {
			return nil
		}
	}
	return errors.New("The server certificate was not valid")
}

// MakeContextDialer returns a ContextDialer that only succeeds on connection to a TLS secured address with the pinned fingerprint
func MakeContextDialer(fingerprint string) ContextDialer {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := tls.Dial(network, addr, nil)
		if err != nil {
			return c, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context completed")
		default:
			if err := validateChain(fingerprint, c.ConnectionState()); err != nil {
				return nil, fmt.Errorf("failed to validate certificate chain: %w", err)
			}
		}
		return c, nil
	}
}

// Dialer is a function connecting to the address on the named network
type Dialer func(network, addr string) (net.Conn, error)

// MakeDialer returns a Dialer that only succeeds on connection to a TLS secured address with the pinned fingerprint
func MakeDialer(fingerprint string) Dialer {
	return func(network, addr string) (net.Conn, error) {
		c, err := tls.Dial(network, addr, nil)
		if err != nil {
			return c, err
		}
		if err := validateChain(fingerprint, c.ConnectionState()); err != nil {
			return nil, fmt.Errorf("failed to validate certificate chain: %w", err)
		}
		return c, nil
	}
}

// GetFingerprints is a helper for getting the fingerprint needed to update pins
func GetFingerprints(c *tls.Conn) (map[string]string, error) {
	connstate := c.ConnectionState()

	if len(connstate.VerifiedChains) < 1 {
		return nil, errors.New("No valid verified chain found")
	}
	prints := make(map[string]string)

	for _, chain := range connstate.VerifiedChains {
		leafCert := chain[0]
		hash := sha256.Sum256(leafCert.RawSubjectPublicKeyInfo)
		digest := base64.StdEncoding.EncodeToString(hash[:])
		prints[leafCert.Issuer.String()] = digest
	}

	return prints, nil
}
