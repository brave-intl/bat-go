GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date +%s)
VAULT_VERSION=0.10.1
TEST_PKG?=./...
TEST_FLAGS= --tags=$(TEST_TAGS) $(TEST_PKG)
ifdef TEST_RUN
	TEST_FLAGS = --tags=$(TEST_TAGS) $(TEST_PKG) --run=$(TEST_RUN)
endif

.PHONY: all buildcmd docker test create-json-schema lint clean
all: test create-json-schema buildcmd

.DEFAULT: buildcmd

buildcmd:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -ldflags "-w -s -X main.version=${GIT_VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${GIT_COMMIT}" -o bat-go main.go

mock:
	mockgen -source=./promotion/claim.go -destination=promotion/mockclaim.go -package=promotion
	mockgen -source=./promotion/drain.go -destination=promotion/mockdrain.go -package=promotion
	mockgen -source=./promotion/datastore.go -destination=promotion/mockdatastore.go -package=promotion
	mockgen -source=./promotion/service.go -destination=promotion/mockservice.go -package=promotion
	mockgen -source=./grant/datastore.go -destination=grant/mockdatastore.go -package=grant
	mockgen -source=./utils/clients/ratios/client.go -destination=utils/clients/ratios/mock/mock.go -package=mock_ratios
	mockgen -source=./utils/clients/cbr/client.go -destination=utils/clients/cbr/mock/mock.go -package=mock_cbr
	mockgen -source=./utils/clients/reputation/client.go -destination=utils/clients/reputation/mock/mock.go -package=mock_reputation
	mockgen -source=./utils/clients/gemini/client.go -destination=utils/clients/gemini/mock/mock.go -package=mock_gemini
	mockgen -source=./utils/clients/bitflyer/client.go -destination=utils/clients/bitflyer/mock/mock.go -package=mock_bitflyer
	mockgen -source=./utils/clients/coingecko/client.go -destination=utils/clients/coingecko/mock/mock.go -package=mock_coingecko
	mockgen -source=./utils/clients/coingecko/assets/client.go -destination=utils/clients/coingecko/assets/mock/mock.go -package=mock_assets
	mockgen -source=./utils/backoff/retrypolicy/retrypolicy.go -destination=utils/backoff/retrypolicy/mock/retrypolicy.go -package=mockretrypolicy

instrumented:
	gowrap gen -p github.com/brave-intl/bat-go/grant -i Datastore -t ./.prom-gowrap.tmpl -o ./grant/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/grant -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./grant/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/promotion -i Datastore -t ./.prom-gowrap.tmpl -o ./promotion/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/promotion -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./promotion/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/skus -i Datastore -t ./.prom-gowrap.tmpl -o ./skus/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/wallet -i Datastore -t ./.prom-gowrap.tmpl -o ./wallet/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/wallet -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./wallet/instrumented_read_only_datastore.go
	# fix everything called datastore...
	sed -i'bak' 's/datastore_duration_seconds/grant_datastore_duration_seconds/g' grant/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/grant_readonly_datastore_duration_seconds/g' ./grant/instrumented_read_only_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/promotion_datastore_duration_seconds/g' ./promotion/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/promotion_readonly_datastore_duration_seconds/g' ./promotion/instrumented_read_only_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/skus_datastore_duration_seconds/g' ./skus/instrumented_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/wallet_datastore_duration_seconds/g' ./wallet/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/wallet_readonly_datastore_duration_seconds/g' ./wallet/instrumented_read_only_datastore.go
	# http clients
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/cbr -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/cbr/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/ratios -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/ratios/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/reputation -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/reputation/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/gemini -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/gemini/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/bitflyer -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/bitflyer/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/coingecko -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/coingecko/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/utils/clients/coingecko/assets -i Client -t ./.prom-gowrap.tmpl -o ./utils/clients/coingecko/assets/instrumented_client.go
	# fix all instrumented cause the interfaces are all called "client"
	sed -i'bak' 's/client_duration_seconds/cbr_client_duration_seconds/g' utils/clients/cbr/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/ratios_client_duration_seconds/g' utils/clients/ratios/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/reputation_client_duration_seconds/g' utils/clients/reputation/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/gemini_client_duration_seconds/g' utils/clients/gemini/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/bitflyer_client_duration_seconds/g' utils/clients/bitflyer/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/coingecko_client_duration_seconds/g' utils/clients/coingecko/instrumented_client.go

%-docker: docker
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t $*-api:latest -f ./$*/Dockerfile .
	docker tag $*-api:latest $*-api:$(GIT_VERSION)

docker:
	docker rmi -f bat-go:latest
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t bat-go:$(GIT_VERSION)$(BUILD_TIME) .
	docker tag bat-go:$(GIT_VERSION)$(BUILD_TIME) bat-go:latest

docker-reproducible:
	docker run -v $(PWD):/workspace --network=host \
		gcr.io/kaniko-project/executor:latest \
		--reproducible --dockerfile /workspace/Dockerfile \
		--no-push --tarPath /workspace/bat-go-repro.tar \
		--destination bat-go-repro:latest --context dir:///workspace/ && cat bat-go-repro.tar | docker load

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
	go run main.go generate json-schema

docker-dev:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.dev.yml run --rm -p 3333:3333 dev /bin/bash

docker-refresh-dev:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.dev-refresh.yml up -d dev-refresh

docker-refresh-skus:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.skus-refresh.yml up -d skus-refresh

settlement-tools:
	$(eval GOOS?=darwin)
	$(eval GOARCH?=amd64)
	rm -rf target/settlement-tools
	mkdir -p target/settlement-tools
	cp settlement/config.hcl target/settlement-tools/
	cp settlement/README.md target/settlement-tools/
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -ldflags "-w -s -X main.version=${GIT_VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${GIT_COMMIT}" -o target/settlement-tools/bat-cli
	GOOS=$(GOOS) GOARCH=$(GOARCH) make download-vault

docker-settlement-tools:
	docker rmi -f brave/settlement-tools:latest
	docker build -f settlement/Dockerfile --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t brave/settlement-tools:$(GIT_VERSION)$(BUILD_TIME) .
	docker tag brave/settlement-tools:$(GIT_VERSION)$(BUILD_TIME) brave/settlement-tools:latest

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

vault:
	./target/settlement-tools/vault server -config=./target/settlement-tools/config.hcl

vault-clean:
	rm -rf share-0.gpg target/settlement-tools/vault-data vault-data

json-schema:
	go run main.go generate json-schema --overwrite

create-json-schema:
	go run main.go generate json-schema

test:
	GODEBUG=x509ignoreCN=0 go test -count 1 -v -p 1 $(TEST_FLAGS)

format:
	gofmt -s -w ./

format-lint:
	make format && make lint
lint:
	golangci-lint run -E gofmt -E revive --exclude-use-default=false
