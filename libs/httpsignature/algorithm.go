package httpsignature

import (
	"errors"
)

// Algorithm is an enum-like representing an algorithm that can be used for http signatures
type Algorithm int

const (
	invalid Algorithm = iota
	// ED25519 EdDSA - deprecated, all algorithm strings should be replaced with HS2019
	ED25519
	// HS2019 is a catch-all value for all algorithms
	HS2019
	// AWSNITRO uses AWS nitro attesation functionality
	AWSNITRO
)

var algorithmName = map[Algorithm]string{
	ED25519:  "ed25519",
	HS2019:   "hs2019",
	AWSNITRO: "awsnitro",
}

var algorithmID = map[string]Algorithm{
	"ed25519":  ED25519,
	"hs2019":   HS2019,
	"awsnitro": AWSNITRO,
}

func (a Algorithm) String() string {
	return algorithmName[a]
}

// MarshalText marshalls the algorithm into text.
func (a *Algorithm) MarshalText() (text []byte, err error) {
	if *a == invalid {
		return nil, errors.New("not a supported algorithm")
	}
	text = []byte(a.String())
	return
}

// UnmarshalText unmarshalls the algorithm from text.
func (a *Algorithm) UnmarshalText(text []byte) (err error) {
	var exists bool
	*a, exists = algorithmID[string(text)]
	if !exists {
		return errors.New("not a supported algorithm")
	}
	return nil
}
