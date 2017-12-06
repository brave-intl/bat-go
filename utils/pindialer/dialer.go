package pindialer

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net"
)

type Dialer func(network, addr string) (net.Conn, error)

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
