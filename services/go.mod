module github.com/brave-intl/bat-go/services

go 1.18

replace github.com/brave-intl/bat-go => ../

replace github.com/brave-intl/bat-go/libs => ../libs

replace github.com/brave-intl/bat-go/tools => ../tools

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/aws/aws-sdk-go-v2/config v1.17.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.27.5
	github.com/aws/smithy-go v1.12.1
	github.com/brave-intl/bat-go v1.0.2
	github.com/brave-intl/bat-go/libs v1.0.2
	github.com/brave-intl/bat-go/tools v1.0.2
	github.com/getsentry/sentry-go v0.13.0
	github.com/go-chi/chi v4.1.2+incompatible
	github.com/go-chi/cors v1.2.1
	github.com/golang-migrate/migrate/v4 v4.15.2
	github.com/golang/mock v1.6.0
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/jmoiron/sqlx v1.3.5
	github.com/lib/pq v1.10.6
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/prometheus/client_golang v1.13.0
	github.com/rs/zerolog v1.28.0
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.4.34
	github.com/shopspring/decimal v1.3.1
	github.com/spf13/cobra v1.5.0
	github.com/spf13/viper v1.12.0
	github.com/stretchr/testify v1.8.0
	github.com/stripe/stripe-go/v72 v72.122.0
	golang.org/x/crypto v0.0.0-20220817201139-bc19a97f63c8
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/square/go-jose.v2 v2.6.0
)

require (
	github.com/aws/aws-sdk-go-v2 v1.16.11 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.4 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.12.14 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.19 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.0.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.13.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.16.13 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-3 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/klauspost/compress v1.15.7 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mssola/user_agent v0.5.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/shengdoushi/base58 v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/square/go-jose v2.6.0+incompatible // indirect
	github.com/subosito/gotenv v1.3.0 // indirect
	github.com/throttled/throttled v2.2.5+incompatible // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/net v0.0.0-20220812174116-3211cb980234 // indirect
	golang.org/x/sys v0.0.0-20220728004956-3c1f35247d10 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/ini.v1 v1.66.4 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
