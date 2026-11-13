package model

type Error string

func (e Error) Error() string {
	return string(e)
}

// CardBytes is the raw JSON payload of the rewards cards.
type CardBytes []byte
