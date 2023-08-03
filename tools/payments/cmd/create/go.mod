module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.13.5
)

require (
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
)
