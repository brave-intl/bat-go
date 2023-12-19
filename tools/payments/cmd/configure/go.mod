module github.com/brave-intl/bat-go/tools/payments/cmd/configure

replace github.com/brave-intl/bat-go/tools/payments => ../../

replace github.com/brave-intl/bat-go/libs => ../../../../libs

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/aws/aws-sdk-go v1.45.21
	github.com/aws/aws-sdk-go-v2/config v1.18.43
	github.com/aws/aws-sdk-go-v2/service/kms v1.24.5
	github.com/aws/aws-sdk-go-v2/service/s3 v1.40.0
)

require (
	github.com/aws/aws-sdk-go-v2 v1.21.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.13 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.41 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.41 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.35 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.43 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.15.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.15.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.17.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.23.0 // indirect
	github.com/aws/smithy-go v1.14.2 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
)
