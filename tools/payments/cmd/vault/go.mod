module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

replace github.com/brave-intl/bat-go/libs => ../../../../libs

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.13.7
)

require (
	filippo.io/edwards25519 v1.0.0 // indirect
	golang.org/x/crypto v0.9.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
)
