// Package pindialer contains a basic implementation of a pinned HTTPS dialer
package pindialer

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
)

// Dialer is a function connecting to the address on the named network
type Dialer func(network, addr string) (net.Conn, error)

// MakeDialer returns a Dialer that only succeeds on connection to a TLS secured address with the pinned fingerprint
func MakeDialer(fingerprint string) Dialer {
	return func(network, addr string) (net.Conn, error) {
		c, err := tls.Dial(network, addr, nil)
		if err != nil {
			return c, err
		}
		connstate := c.ConnectionState()
		for _, chain := range connstate.VerifiedChains {
			// allow for intermediate certificate pinning, or leaf certificate pinning
			for _, node := range chain {
				hash := sha256.Sum256(node.RawSubjectPublicKeyInfo)
				digest := base64.StdEncoding.EncodeToString(hash[:])
				if digest == fingerprint {
					return c, nil
				}
			}
		}
		return c, errors.New("The server certificate was not valid")
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
		for _, node := range chain {
			hash := sha256.Sum256(node.RawSubjectPublicKeyInfo)
			digest := base64.StdEncoding.EncodeToString(hash[:])
			prints[node.Issuer.String()] = digest
		}
	}

	return prints, nil
}
