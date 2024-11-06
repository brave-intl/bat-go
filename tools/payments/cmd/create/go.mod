module github.com/brave-intl/bat-go/tools/payments/cmd/create

go 1.22.5

toolchain go1.22.7

replace github.com/brave-intl/bat-go/tools/payments => ../../

replace github.com/pires/go-proxyproto v1.0.0 => github.com/pires/go-proxyproto v0.8.0

require (
	filippo.io/age v1.1.1
	github.com/hashicorp/vault v1.18.1
)

require (
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
)
