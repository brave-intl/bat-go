package cryptography

// Presignable duplicates the hmac interface for signing
type Presignable interface {
	HMACSha384(payload []byte) ([]byte, error)
}

// Presigner returns the same value always
type Presigner struct {
	sig []byte
}

// HMACSha384 presigns a request
func (ps Presigner) HMACSha384(payload []byte) ([]byte, error) {
	return ps.sig, nil
}

// NewPresigner creates a new presigner
func NewPresigner(sig []byte) Presignable {
	return Presigner{sig}
}
