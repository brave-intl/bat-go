// Package pindialer contains a basic implementation of a pinned HTTPS dialer
package pindialer

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
)

func validateChain(fingerprint string, verifiedChains [][]*x509.Certificate) error {
	for _, chain := range verifiedChains {
		for _, cert := range chain {
			hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			digest := base64.StdEncoding.EncodeToString(hash[:])
			if digest == fingerprint {
				return nil
			}
		}
	}
	return errors.New("the server certificate was not valid")
}

// Get tls.Config that validates the connection certificate chain against the
// given fingerprint.
func GetTLSConfig(fingerprint string) *tls.Config {
	return &tls.Config{
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return validateChain(fingerprint, verifiedChains)
		},
	}
}

// GetFingerprints is a helper for getting the fingerprint needed to update pins
func GetFingerprints(c *tls.Conn) (map[string]string, error) {
	connstate := c.ConnectionState()

	if len(connstate.VerifiedChains) < 1 {
		return nil, errors.New("no valid verified chain found")
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
