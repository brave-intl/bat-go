module github.com/brave-intl/bat-go/services

go 1.21

replace github.com/brave-intl/bat-go/cmd => ../cmd

replace github.com/brave-intl/bat-go/libs => ../libs

replace github.com/brave-intl/bat-go/tools => ../tools

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/awa/go-iap v1.3.22
	github.com/aws/aws-sdk-go-v2/service/s3 v1.31.0
	github.com/brave-intl/bat-go v1.0.2
	github.com/brave-intl/bat-go/libs v1.0.2
	github.com/brave-intl/bat-go/tools v1.0.2
	github.com/btcsuite/btcutil v1.0.2
	github.com/getsentry/sentry-go v0.14.0
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-chi/cors v1.2.1
	github.com/go-jose/go-jose/v3 v3.0.1
	github.com/go-playground/validator/v10 v10.14.1
	github.com/golang-migrate/migrate/v4 v4.15.2
	github.com/golang/mock v1.6.0
	github.com/jmoiron/sqlx v1.3.5
	github.com/lib/pq v1.10.9
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.13.0
	github.com/redis/go-redis/v9 v9.7.0
	github.com/rs/zerolog v1.28.0
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.4.35
	github.com/shopspring/decimal v1.3.1
	github.com/spf13/cobra v1.6.1
	github.com/spf13/viper v1.13.0
	github.com/square/go-jose v2.6.0+incompatible
	github.com/stretchr/testify v1.8.4
	github.com/stripe/stripe-go/v72 v72.122.0
	google.golang.org/api v0.134.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/square/go-jose.v2 v2.6.0
)

require (
	cloud.google.com/go/compute v1.21.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/aws/aws-sdk-go-v2 v1.17.7 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.18.19 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.0.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.14.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.18.7 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.5 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/klauspost/compress v1.15.15 // indirect
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mssola/user_agent v0.5.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.17 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/shengdoushi/base58 v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.4.1 // indirect
	github.com/throttled/throttled/v2 v2.12.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/oauth2 v0.11.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230803162519-f966b187b2e5 // indirect
	google.golang.org/grpc v1.58.3 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
