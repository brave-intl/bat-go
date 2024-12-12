module github.com/brave-intl/bat-go/tools/payments/cmd/create

replace github.com/brave-intl/bat-go/tools/payments => ../../

go 1.22.1

require (
	filippo.io/age v1.1.1
	github.com/openbao/openbao/sdk/v2 v2.0.1
)

require (
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)
