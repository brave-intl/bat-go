module github.com/brave-intl/bat-go/services

go 1.18

replace github.com/brave-intl/bat-go/cmd => ../cmd

replace github.com/brave-intl/bat-go/libs => ../libs

replace github.com/brave-intl/bat-go/tools => ../tools

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/awa/go-iap v1.3.19
	github.com/aws/aws-sdk-go-v2/service/s3 v1.27.9
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
	github.com/lib/pq v1.10.7
	github.com/linkedin/goavro v2.1.0+incompatible
	github.com/prometheus/client_golang v1.13.0
	github.com/rs/zerolog v1.28.0
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.4.35
	github.com/shopspring/decimal v1.3.1
	github.com/spf13/cobra v1.5.0
	github.com/spf13/viper v1.13.0
	github.com/stretchr/testify v1.8.0
	github.com/stripe/stripe-go/v72 v72.122.0
	golang.org/x/crypto v0.0.0-20220829220503-c86fa9a7ed90
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/square/go-jose.v2 v2.6.0
)

require (
	cloud.google.com/go/compute v1.6.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.16.15 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.7 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.17.5 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.12.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.0.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.13.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.13.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.16.17 // indirect
	github.com/aws/smithy-go v1.13.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/googleapis/gax-go/v2 v2.4.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-3 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mssola/user_agent v0.5.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.5 // indirect
	github.com/pierrec/lz4/v4 v4.1.16 // indirect
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
	github.com/subosito/gotenv v1.4.1 // indirect
	github.com/throttled/throttled v2.2.5+incompatible // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/net v0.0.0-20220909164309-bea034e7d591 // indirect
	golang.org/x/oauth2 v0.0.0-20220411215720-9780585627b5 // indirect
	golang.org/x/sys v0.0.0-20220913153101-76c7481b5158 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/api v0.81.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220519153652-3a47de7e79bd // indirect
	google.golang.org/grpc v1.46.2 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
