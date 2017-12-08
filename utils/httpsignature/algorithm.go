package httpsignature

import (
	"errors"
)

// Algorithm is an enum-like representing an algorithm that can be used for http signatures
type Algorithm int

const (
	invalid Algorithm = iota
	// ED25519 EdDSA
	ED25519
)

var algorithmName = map[Algorithm]string{
	ED25519: "ed25519",
}

var algorithmID = map[string]Algorithm{
	"ed25519": ED25519,
}

func (a Algorithm) String() string {
	return algorithmName[a]
}

// MarshalText marshalls the algorithm into text.
func (a *Algorithm) MarshalText() (text []byte, err error) {
	if *a == invalid {
		return nil, errors.New("Not a supported algorithm")
	}
	text = []byte(a.String())
	return
}

// UnmarshalText unmarshalls the algorithm from text.
func (a *Algorithm) UnmarshalText(text []byte) (err error) {
	var exists bool
	*a, exists = algorithmID[string(text)]
	if !exists {
		return errors.New("Not a supported algorithm")
	}
	return nil
}
