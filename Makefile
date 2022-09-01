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
	mockgen -source=./services/promotion/claim.go -destination=services/promotion/mockclaim.go -package=promotion
	mockgen -source=./services/promotion/drain.go -destination=services/promotion/mockdrain.go -package=promotion
	mockgen -source=./services/promotion/datastore.go -destination=services/promotion/mockdatastore.go -package=promotion
	mockgen -source=./services/promotion/service.go -destination=services/promotion/mockservice.go -package=promotion
	mockgen -source=./services/grant/datastore.go -destination=services/grant/mockdatastore.go -package=grant
	mockgen -source=./services/wallet/service.go -destination=services/wallet/mockservice.go -package=wallet
	mockgen -source=./services/skus/datastore.go -destination=services/skus/mockdatastore.go -package=skus
	mockgen -source=./libs/clients/ratios/client.go -destination=libs/clients/ratios/mock/mock.go -package=mock_ratios
	mockgen -source=./libs/clients/cbr/client.go -destination=libs/clients/cbr/mock/mock.go -package=mock_cbr
	mockgen -source=./libs/clients/reputation/client.go -destination=libs/clients/reputation/mock/mock.go -package=mock_reputation
	mockgen -source=./libs/clients/gemini/client.go -destination=libs/clients/gemini/mock/mock.go -package=mock_gemini
	mockgen -source=./libs/clients/bitflyer/client.go -destination=libs/clients/bitflyer/mock/mock.go -package=mock_bitflyer
	mockgen -source=./libs/clients/coingecko/client.go -destination=libs/clients/coingecko/mock/mock.go -package=mock_coingecko
	mockgen -source=./libs/backoff/retrypolicy/retrypolicy.go -destination=libs/backoff/retrypolicy/mock/retrypolicy.go -package=mockretrypolicy
	mockgen -source=./libs/aws/s3.go -destination=libs/aws/mock/mock.go -package=mockaws

instrumented:
	gowrap gen -p github.com/brave-intl/bat-go/services/grant -i Datastore -t ./.prom-gowrap.tmpl -o ./services/grant/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/grant -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./services/grant/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/promotion -i Datastore -t ./.prom-gowrap.tmpl -o ./services/promotion/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/promotion -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./services/promotion/instrumented_read_only_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/skus -i Datastore -t ./.prom-gowrap.tmpl -o ./services/skus/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/wallet -i Datastore -t ./.prom-gowrap.tmpl -o ./services/wallet/instrumented_datastore.go
	gowrap gen -p github.com/brave-intl/bat-go/services/wallet -i ReadOnlyDatastore -t ./.prom-gowrap.tmpl -o ./services/wallet/instrumented_read_only_datastore.go
	# fix everything called datastore...
	sed -i'bak' 's/datastore_duration_seconds/grant_datastore_duration_seconds/g' services/grant/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/grant_readonly_datastore_duration_seconds/g' ./services/grant/instrumented_read_only_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/promotion_datastore_duration_seconds/g' ./services/promotion/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/promotion_readonly_datastore_duration_seconds/g' ./services/promotion/instrumented_read_only_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/skus_datastore_duration_seconds/g' ./services/skus/instrumented_datastore.go
	sed -i'bak' 's/datastore_duration_seconds/wallet_datastore_duration_seconds/g' ./services/wallet/instrumented_datastore.go
	sed -i'bak' 's/readonlydatastore_duration_seconds/wallet_readonly_datastore_duration_seconds/g' ./services/wallet/instrumented_read_only_datastore.go
	# http clients
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/cbr -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/cbr/instrumented_client.go
	sed -i'bak' 's/cbr.//g' libs/clients/cbr/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/ratios -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/ratios/instrumented_client.go
	sed -i'bak' 's/ratios.//g' libs/clients/ratios/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/reputation -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/reputation/instrumented_client.go
	sed -i'bak' 's/reputation.//g' libs/clients/reputation/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/gemini -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/gemini/instrumented_client.go
	sed -i'bak' 's/gemini.//g' libs/clients/gemini/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/bitflyer -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/bitflyer/instrumented_client.go
	sed -i'bak' 's/bitflyer.//g' libs/clients/bitflyer/instrumented_client.go
	gowrap gen -p github.com/brave-intl/bat-go/libs/clients/coingecko -i Client -t ./.prom-gowrap.tmpl -o ./libs/clients/coingecko/instrumented_client.go
	sed -i'bak' 's/coingecko.//g' libs/clients/coingecko/instrumented_client.go
	# fix all instrumented cause the interfaces are all called "client"
	sed -i'bak' 's/client_duration_seconds/cbr_client_duration_seconds/g' libs/clients/cbr/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/ratios_client_duration_seconds/g' libs/clients/ratios/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/reputation_client_duration_seconds/g' libs/clients/reputation/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/gemini_client_duration_seconds/g' libs/clients/gemini/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/bitflyer_client_duration_seconds/g' libs/clients/bitflyer/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/coingecko_client_duration_seconds/g' libs/clients/coingecko/instrumented_client.go

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
	go test -count 1 -v -p 1 $(TEST_FLAGS) github.com/brave-intl/bat-go/...

format:
	gofmt -s -w ./

format-lint:
	make format && make lint
lint:
	golangci-lint run -E gofmt -E revive --exclude-use-default=false
