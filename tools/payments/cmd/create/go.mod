module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.14.10
)

require (
	golang.org/x/crypto v0.18.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
)
