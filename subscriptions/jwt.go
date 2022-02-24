package subscriptions

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/kafka"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

const validJWTAlg = jose.RS256

type JwtService interface {
	MakeJWT(c interface{}) (string, error)
	MustVerify(j string, v interface{}) error
}

type jwtModule struct {
	JAASTenantID string
	JAASKeyID    string
	PrivateKey   *rsa.PrivateKey
}

// func (s *Server) initJWTModule() error {
// 	privateKey, err := getPrivKey(s.config.PrivateKey, s.config.PrivateKeyLocation)
// 	if err != nil {
// 		return err
// 	}
// 	s.jwtModule = &jwtModule{
// 		JAASKeyID:    s.config.JAASKeyID,
// 		JAASTenantID: s.config.JAASTenantID,
// 		PrivateKey:   privateKey,
// 	}
// 	return nil
// }

func (s Service) MakeJWT(c interface{}, ctx context.Context) (string, error) {
	privKey, ok := ctx.Value(appctx.JAASPrivateKeyCTXKey).(rsa.PrivateKey)
	if !ok {
		return "", errors.New("Invalid private key in context")
	}

	key := jose.SigningKey{Algorithm: validJWTAlg, Key: privKey}
	var signerOpts = jose.SignerOptions{}
	signerOpts.WithType("JWT")

	JAASTenantID, ok := ctx.Value(appctx.JAASTenantIDCTXKey).(string)
	if !ok {
		return "", errors.New("Invalid JAAS tenant ID in context")
	}

	JAASKeyID, ok := ctx.Value(appctx.JAASKeyIDCTXKey).(string)
	if !ok {
		return "", errors.New("Invalid JASS key ID in context")
	}

	signerOpts.ExtraHeaders = map[jose.HeaderKey]interface{}{
		"kid": JAASTenantID + "/" + JAASKeyID,
		"typ": "JWT",
	}
	rsaSigner, err := jose.NewSigner(key, &signerOpts)
	if err != nil {
		return "", err
	}
	builder := jwt.Signed(rsaSigner)

	builder = builder.Claims(c)

	rawJWT, err := builder.CompactSerialize()
	if err != nil {
		return "", err
	}

	return rawJWT, nil
}

func (s Service) MustVerify(j string, v interface{}, ctx context.Context) error {
	parsedJWT, err := jwt.ParseSigned(j)
	if err != nil {
		return err
	}

	// In order to prevent JWT forgeability this service only supports the "RS256" jwt key type. Algorithms of "none" or
	// any other keytype must be rejected.
	if len(parsedJWT.Headers) != 1 || parsedJWT.Headers[0].Algorithm != string(validJWTAlg) {
		return errors.New("failed to parse JWT token: invalid number of signatures or algorithm specified")
	}

	privKey, ok := ctx.Value(appctx.JAASPrivateKeyCTXKey).(rsa.PrivateKey)
	if !ok {
		return errors.New("Invalid private key in context")
	}

	return parsedJWT.Claims(&privKey.PublicKey, v)
}

func getPrivKey(privateKeyRaw, privatKeyLocation string) (*rsa.PrivateKey, error) {
	var err error
	var priv []byte
	if len(privateKeyRaw) == 0 {
		priv, err = kafka.ReadFileFromEnvLoc("PRIVATE_KEY_LOCATION", true)
		if err != nil {
			privkey, err := rsa.GenerateKey(rand.Reader, 4096)
			if err != nil {
				return nil, err
			}
			return privkey, nil
		}
	} else {
		p, err := base64.StdEncoding.DecodeString(privateKeyRaw)
		if err != nil {
			return nil, err
		}
		priv = p
	}

	privPem, _ := pem.Decode(priv)
	if privPem == nil {
		return nil, errors.New("RSA private key could not be decoded")
	}
	if privPem.Type != "RSA PRIVATE KEY" {
		return nil, errors.New("RSA private key is of the wrong type")
	}
	var privPemBytes = privPem.Bytes

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKCS1PrivateKey(privPemBytes); err != nil {
		if parsedKey, err = x509.ParsePKCS8PrivateKey(privPemBytes); err != nil { // note this returns type `interface{}`
			return nil, err
		}
	}

	var privateKey *rsa.PrivateKey
	var ok bool
	privateKey, ok = parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("Unable to parse RSA private key")
	}

	return privateKey, nil
}
