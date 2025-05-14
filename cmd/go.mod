module github.com/brave-intl/bat-go/cmd

go 1.23.0

toolchain go1.23.9

replace github.com/brave-intl/bat-go/services => ../services

replace github.com/brave-intl/bat-go/libs => ../libs

replace github.com/brave-intl/bat-go/tools => ../tools

require (
	github.com/brave-intl/bat-go/libs v0.0.0-20250514144339-ea3f4051ec9f
	github.com/rs/zerolog v1.28.0
	github.com/spf13/cobra v1.6.1
	github.com/spf13/viper v1.13.0
)

require (
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/getsentry/sentry-go v0.14.0 // indirect
	github.com/go-chi/chi v4.1.2+incompatible // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.5 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/redis/go-redis/v9 v9.8.0 // indirect
	github.com/rs/xid v1.4.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/shengdoushi/base58 v1.0.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.4.1 // indirect
	github.com/throttled/throttled/v2 v2.12.0 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
