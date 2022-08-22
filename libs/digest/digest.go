// Package digest implements an instance which serializes to / from the Digest header per rfc3230
// https://tools.ietf.org/html/rfc3230
package digest

import (
	"crypto"
	_ "crypto/sha256" // Needed since crypto.SHA256 does not actually pull in implementation
	_ "crypto/sha512" // Needed since crypto.SHA512 does not actually pull in implementation
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Instance consists of the hash type and an internal digest value
type Instance struct {
	crypto.Hash
	Digest string
}

var hashName = map[crypto.Hash]string{
	crypto.SHA256: "SHA-256",
	crypto.SHA512: "SHA-512",
}

var hashID = map[string]crypto.Hash{
	"SHA-256": crypto.SHA256,
	"SHA-512": crypto.SHA512,
}

// Return a string representation digest in HTTP Digest header format
func (d *Instance) String() string {
	return fmt.Sprintf("%s=%s", hashName[d.Hash], d.Digest)
}

// MarshalText returns the marshalled digest in HTTP Digest header format
func (d *Instance) MarshalText() (text []byte, err error) {
	return []byte(d.String()), nil
}

// UnmarshalText parses the digest from HTTP Digest header format
func (d *Instance) UnmarshalText(text []byte) (err error) {
	s := strings.SplitN(string(text), "=", 2)
	if len(s) != 2 {
		return errors.New("a valid digest specifier must consist of two parts separated by =")
	}
	var exists bool
	d.Hash, exists = hashID[s[0]]
	if !exists {
		return fmt.Errorf("the digest algorithm %s is not supported", s[0])
	}
	d.Digest = s[1]
	return nil
}

// Calculate but do not update the internal digest value for b.
// Subsequent calls to MarshalText, String, etc will not use the returned value.
func (d *Instance) Calculate(b []byte) string {
	hash := d.New()
	_, err := hash.Write(b)
	if err != nil {
		panic(err)
	}
	var out []byte
	return base64.StdEncoding.EncodeToString(hash.Sum(out))
}

// Update (after calculating) the internal digest value for b.
// Subsequent calls to MarshalText, String, etc will use the updated value.
func (d *Instance) Update(b []byte) {
	d.Digest = d.Calculate(b)
}

// Verify by calculating the digest value for b and checking against the internal digest value.
// Returns true if the digest values match.
func (d *Instance) Verify(b []byte) bool {
	expected := d.Calculate(b)
	return d.Digest == expected
}
