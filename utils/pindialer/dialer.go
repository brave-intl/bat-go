// Package pindialer contains a basic implementation of a pinned HTTPS dialer
package pindialer

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net"
)

// Dialer is a function connecting to the address on the named network
type Dialer func(network, addr string) (net.Conn, error)

// MakeDialer returns a Dialer that only suceeds on connection to a TLS secured address with the pinned fingerprint
func MakeDialer(fingerprint string) Dialer {
	return func(network, addr string) (net.Conn, error) {
		c, err := tls.Dial(network, addr, nil)
		if err != nil {
			return c, err
		}
		connstate := c.ConnectionState()
		for _, chain := range connstate.VerifiedChains {
			leafCert := chain[0]
			hash := sha256.Sum256(leafCert.RawSubjectPublicKeyInfo)
			digest := base64.StdEncoding.EncodeToString(hash[:])
			if digest == fingerprint {
				return c, nil
			}
		}
		panic("The pinned certificate was not valid")
	}
}
