module github.com/brave-intl/bat-go/tools/payments/cmd/bootstrap

replace github.com/brave-intl/bat-go/tools/payments => ../../

replace github.com/brave-intl/bat-go/libs => ../../../../libs

go 1.20

require (
	filippo.io/age v1.1.1
	github.com/aws/aws-sdk-go-v2 v1.30.1
	github.com/aws/aws-sdk-go-v2/config v1.27.23
	github.com/aws/aws-sdk-go-v2/service/kms v1.35.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.57.1
	github.com/brave-intl/bat-go/libs v0.0.0-20230403164253-ad445ff37512
	github.com/brave-intl/bat-go/tools/payments v0.0.0-00010101000000-000000000000
)

require (
	filippo.io/edwards25519 v1.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.3 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.23 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.13 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.13 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.22.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.26.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.30.1 // indirect
	github.com/aws/smithy-go v1.20.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fxamacker/cbor/v2 v2.4.0 // indirect
	github.com/getsentry/sentry-go v0.14.0 // indirect
	github.com/go-chi/chi v4.1.2+incompatible // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/gomodule/redigo v2.0.0+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hf/nitrite v0.0.0-20211104000856-f9e0dcc73703 // indirect
	github.com/hf/nsm v0.0.0-20220930140112-cd181bd646b9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mdlayher/socket v0.4.0 // indirect
	github.com/mdlayher/vsock v1.2.0 // indirect
	github.com/prometheus/client_golang v1.14.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/redis/go-redis/v9 v9.3.0 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/rs/zerolog v1.29.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/shengdoushi/base58 v1.0.0 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/throttled/throttled v2.2.5+incompatible // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sync v0.6.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/term v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
