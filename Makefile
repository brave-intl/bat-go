GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date +%s)
VAULT_VERSION=0.10.1
_BINS := $(wildcard bin/*)

ifdef GOOS
	BINS := $(_BINS:bin/%=target/$(GOOS)_$(GOARCH)/%)
else
	BINS := $(_BINS:bin/%=target/release/%)
endif

TEST_PKG?=./...
TEST_FLAGS= --tags=$(TEST_TAGS) $(TEST_PKG)
ifdef TEST_RUN
	TEST_FLAGS = --tags=$(TEST_TAGS) $(TEST_PKG) --run=$(TEST_RUN)
endif

.PHONY: all bins docker test lint clean
all: test bins

bins: clean $(BINS)

.DEFAULT:
	go build ./bin/$@

target/%:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./bin/$(notdir $@)

instrumented:
	gowrap gen -p github.com/brave-intl/bat-go/grant -i Datastore -t ./.prom-gowrap.tmpl -o ./grant/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/grant -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./grant/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/promotion -i Datastore -t ./.prom-gowrap.tmpl -o ./promotion/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/promotion -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./promotion/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/payment -i Datastore -t ./.prom-gowrap.tmpl -o ./payment/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/wallet/service -i Datastore -t ./.prom-gowrap.tmpl -o ./wallet/service/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/wallet/service -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./wallet/service/instrumented_read_only_datastore.go
	# fix everything called datastore...
	sed -i 's/datastore_duration_seconds/grant_datastore_duration_seconds/g' ./grant/instrumented_datastore.go
	sed -i 's/readonlydatastore_duration_seconds/grant_readonly_datastore_duration_seconds/g' ./grant/instrumented_read_only_datastore.go
	sed -i 's/datastore_duration_seconds/promotion_datastore_duration_seconds/g' ./promotion/instrumented_datastore.go
	sed -i 's/readonlydatastore_duration_seconds/promotion_readonly_datastore_duration_seconds/g' ./promotion/instrumented_read_only_datastore.go
	sed -i 's/datastore_duration_seconds/payment_datastore_duration_seconds/g' ./payment/instrumented_datastore.go
	sed -i 's/datastore_duration_seconds/wallet_datastore_duration_seconds/g' ./wallet/service/instrumented_datastore.go
	sed -i 's/readonlydatastore_duration_seconds/wallet_readonly_datastore_duration_seconds/g' ./wallet/service/instrumented_read_only_datastore.go
	# http clients
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/balance -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/balance/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/cbr -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/cbr/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/ledger -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/ledger/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/ratios -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/ratios/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/reputation -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/reputation/instrumented_client.go
	# fix all instrumented cause the interfaces are all called "client"
	sed -i 's/client_duration_seconds/cbr_client_duration_seconds/g' utils/clients/cbr/instrumented_client.go
	sed -i 's/client_duration_seconds/balance_client_duration_seconds/g' utils/clients/balance/instrumented_client.go
	sed -i 's/client_duration_seconds/ledger_client_duration_seconds/g' utils/clients/ledger/instrumented_client.go
	sed -i 's/client_duration_seconds/ratios_client_duration_seconds/g' utils/clients/ratios/instrumented_client.go
	sed -i 's/client_duration_seconds/reputation_client_duration_seconds/g' utils/clients/reputation/instrumented_client.go

rewards-docker:
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t rewards-api:latest -f ./rewards/Dockerfile .
	docker tag rewards-api:latest rewards-api:$(GIT_VERSION)

docker:
	docker rmi -f bat-go:latest
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t bat-go:$(GIT_VERSION)$(BUILD_TIME) .
	docker tag bat-go:$(GIT_VERSION)$(BUILD_TIME) bat-go:latest

docker-up-dev:
	COMMIT=$(GIT_COMMIT) VERSION=$(GIT_VERSION) BUILD_TIME=$(BUILD_TIME) docker-compose \
		-f docker-compose.yml -f docker-compose.dev.yml up -d

docker-up-dev-rep:
	COMMIT=$(GIT_COMMIT) VERSION=$(GIT_VERSION) BUILD_TIME=$(BUILD_TIME) docker-compose \
		-f docker-compose.yml -f docker-compose.reputation.yml -f docker-compose.dev.yml up -d

docker-test:
	COMMIT=$(GIT_COMMIT) VERSION=$(GIT_VERSION) BUILD_TIME=$(BUILD_TIME) docker-compose \
		-f docker-compose.yml -f docker-compose.dev.yml up -d vault
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) PKG=$(TEST_PKG) RUN=$(TEST_RUN) docker-compose -f docker-compose.yml -f docker-compose.dev.yml run --rm dev make test

docker-dev:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.dev.yml run --rm dev /bin/bash

docker-refresh-dev:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.dev-refresh.yml up -d dev-refresh

mac:
	GOOS=darwin GOARCH=amd64 make bins

settlement-tools:
	$(eval GOOS?=darwin)
	$(eval GOARCH?=amd64)
	rm -rf target/settlement-tools
	mkdir -p target/settlement-tools
	cp settlement/config.hcl target/settlement-tools/
	cp settlement/README.md target/settlement-tools/
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/vault-init
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/vault-unseal
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/vault-import-key
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/vault-create-wallet
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/vault-sign-settlement
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/settlement-submit
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o target/settlement-tools/bat-cli
	GOOS=$(GOOS) GOARCH=$(GOARCH) make download-vault

grant-signing-tools:
	$(eval GOOS?=darwin)
	$(eval GOARCH?=amd64)
	rm -rf target/grant-signing-tools
	mkdir -p target/grant-signing-tools
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/grant-signing-tools/create-tokens
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/grant-signing-tools/create-tokens-ads
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/grant-signing-tools/verify-tokens

download-vault:
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_$(GOOS)_$(GOARCH).zip
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_SHA256SUMS
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_SHA256SUMS.sig
	cd target/settlement-tools && gpg --verify vault_$(VAULT_VERSION)_SHA256SUMS.sig vault_$(VAULT_VERSION)_SHA256SUMS
	cd target/settlement-tools && grep $(GOOS)_$(GOARCH) vault_$(VAULT_VERSION)_SHA256SUMS | shasum -a 256 -c
	cd target/settlement-tools && unzip -o vault_$(VAULT_VERSION)_$(GOOS)_$(GOARCH).zip vault && rm vault_$(VAULT_VERSION)_*

test:
	go test -v -p 1 $(TEST_FLAGS)

format:
	gofmt -s -w ./
format-lint:
	make format && make lint
lint:
	golangci-lint run -E gofmt -E golint --exclude-use-default=false

clean:
	rm -f $(BINS)
