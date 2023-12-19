package payments

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/libs/prompt"
	"golang.org/x/crypto/ssh"
)

// GetOperatorPrivateKey - get the private key from the file specified
func GetOperatorPrivateKey(filename string) (ed25519.PrivateKey, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open key file: %w", err)
	}
	defer f.Close()

	privateKeyPEM, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var key interface{}
	if strings.HasPrefix(string(privateKeyPEM), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		pass, err := prompt.Password()
		if err != nil {
			return nil, fmt.Errorf("failed to read password from terminal: %w", err)
		}

		key, err = ssh.ParseRawPrivateKeyWithPassphrase(privateKeyPEM, pass)
		if err != nil {
			return nil, fmt.Errorf("failed to read ssh key file: %w", err)
		}
	} else {
		p, _ := pem.Decode(privateKeyPEM)
		if p != nil {
			key, err = x509.ParsePKCS8PrivateKey(p.Bytes)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("key is not valid pem")
		}
	}

	var edKey ed25519.PrivateKey
	switch k := key.(type) {
	case *ed25519.PrivateKey:
		edKey = *k
	case ed25519.PrivateKey:
		edKey = k
	default:
		return nil, fmt.Errorf("key is not ed25519 key")
	}

	return edKey, nil
}
