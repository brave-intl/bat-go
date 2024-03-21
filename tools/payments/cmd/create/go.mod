module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

replace github.com/brave-intl/bat-go/libs => ../../../../libs

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.13.12
)

require (
	golang.org/x/crypto v0.15.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
)
