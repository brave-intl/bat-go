package payments

import "os"

const (
	jegan      = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDfcr9jUEu9D9lSpUnPwT1cCggCe48kZw1bJt+CXYSnh jegan+settlements@brave.com"
	jeganDev   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMKhViUd6Nwd8qre0go7Qc6Wa6Q7A3GiWj7q/GMF/NzV jegan+devsettlements@brave.com"
	evq        = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA91/jZI+hcisdAURdqgdAKyetA4b2mVJIypfEtTyXW+ evq+settlements@brave.com"
	kdenhartog = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEY/3VGKsrH5dp3mK5PJIHVkUMWpsmUhZkrLuZTf7Sqr kdenhartog+settlement+dev@brave.com"
	jtieman    = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK1fxpURIUAJNRqosAnPPXnKjpUBGGOKgkUOXmviJfFx jtieman+nitro@brave.com"
)

// vaultManagerKeys returns the set of keys permitted to interact with the secrets vault.
func vaultManagerKeys() []string {
	switch os.Getenv("ENV") {
	case "staging":
		return []string{jegan, evq}
	case "development":
		return []string{jegan, evq, jeganDev}
	default:
		return nil
	}
}

// paymentOperatorKeys returns the set of keys permitted to interact with transactions. This set
// includes at least the vault manager keys.
func paymentOperatorKeys() []string {
	vaultManagers := vaultManagerKeys()
	switch os.Getenv("ENV") {
	case "staging":
		return append(vaultManagers, jtieman)
	case "development":
		return append(vaultManagers, jtieman, kdenhartog)
	default:
		return nil
	}
}
