package httpsignature

import (
	"errors"
)

type Algorithm int

const (
	INVALID Algorithm = iota
	ED25519
)

var algorithmName = map[Algorithm]string{
	ED25519: "ed25519",
}

var algorithmId = map[string]Algorithm{
	"ed25519": ED25519,
}

func (a Algorithm) String() string {
	return algorithmName[a]
}

func (a *Algorithm) MarshalText() (text []byte, err error) {
	if *a == INVALID {
		return nil, errors.New("Not a supported algorithm")
	}
	text = []byte(a.String())
	return
}

func (a *Algorithm) UnmarshalText(text []byte) (err error) {
	s := string(text)
	var exists bool
	*a, exists = algorithmId[s]
	if !exists {
		return errors.New("Not a supported algorithm")
	}
	return nil
}
