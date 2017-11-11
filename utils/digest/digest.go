package digest

import (
	"crypto"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type DigestInstance struct {
	crypto.Hash
	Digest string
}

var hashName = map[crypto.Hash]string{
	crypto.SHA256: "SHA-256",
	crypto.SHA512: "SHA-512",
}

var hashId = map[string]crypto.Hash{
	"SHA-256": crypto.SHA256,
	"SHA-512": crypto.SHA512,
}

func (d *DigestInstance) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprintf("%s=%s", hashName[d.Hash], d.Digest)), nil
}

func (d *DigestInstance) UnmarshalText(text []byte) (err error) {
	s := strings.SplitN(string(text), "=", 2)
	if len(s) != 2 {
		return errors.New("A valid digest specifier must consist of two parts separated by =")
	}
	var exists bool
	d.Hash, exists = hashId[s[0]]
	if !exists {
		return errors.New(fmt.Sprintf("The digest algorithm %s is not supported", s[0]))
	}
	d.Digest = s[1]
	return nil
}

func (d *DigestInstance) Calculate(b []byte) string {
	hash := d.New()
	hash.Write(b)
	var out []byte
	return base64.StdEncoding.EncodeToString(hash.Sum(out))
}

func (d *DigestInstance) Verify(b []byte) bool {
	expected := d.Calculate(b)
	if d.Digest != expected {
		return false
	}
	return true
}
