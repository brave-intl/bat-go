module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

go 1.19

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.12.3
)

require (
	golang.org/x/crypto v0.4.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
)
