GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date +%s)

ifeq (${OUTPUT_DIR},)
	OUTPUT := ..
else
	OUTPUT := ${OUTPUT_DIR}
endif


VAULT_VERSION=0.10.1
TEST_PKG?=./...
TEST_FLAGS= --tags=$(TEST_TAGS) $(TEST_PKG)
ifdef TEST_RUN
	TEST_FLAGS = --tags=$(TEST_TAGS) $(TEST_PKG) --run=$(TEST_RUN)
endif

.PHONY: all buildcmd docker test create-json-schema lint clean download-mod
all: test create-json-schema buildcmd

.DEFAULT: buildcmd

codeql: download-mod buildcmd

buildcmd:
	cd main && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-w -s -X main.version=${GIT_VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${GIT_COMMIT}" -o ${OUTPUT}/bat-go main.go

mock:
	cd services && mockgen -source=./promotion/claim.go -destination=promotion/mockclaim.go -package=promotion
	cd services && mockgen -source=./promotion/drain.go -destination=promotion/mockdrain.go -package=promotion
	cd services && mockgen -source=./promotion/datastore.go -destination=promotion/mockdatastore.go -package=promotion
	cd services && mockgen -source=./promotion/service.go -destination=promotion/mockservice.go -package=promotion
	cd services && mockgen -source=./grant/datastore.go -destination=grant/mockdatastore.go -package=grant
	cd services && mockgen -source=./wallet/service.go -destination=wallet/mockservice.go -package=wallet
	cd services && mockgen -source=./skus/datastore.go -destination=skus/mockdatastore.go -package=skus
	cd services && mockgen -source=./skus/credentials.go -destination=skus/mockcredentials.go -package=skus
	cd libs && mockgen -source=./clients/ratios/client.go -destination=clients/ratios/mock/mock.go -package=mock_ratios
	cd libs && mockgen -source=./clients/cbr/client.go -destination=clients/cbr/mock/mock.go -package=mock_cbr
	cd libs && mockgen -source=./clients/reputation/client.go -destination=clients/reputation/mock/mock.go -package=mock_reputation
	cd libs && mockgen -source=./clients/gemini/client.go -destination=clients/gemini/mock/mock.go -package=mock_gemini
	cd libs && mockgen -source=./clients/radom/client.go -destination=clients/radom/mock/mock.go -package=mock_radom
	cd libs && mockgen -source=./clients/bitflyer/client.go -destination=clients/bitflyer/mock/mock.go -package=mock_bitflyer
	cd libs && mockgen -source=./clients/coingecko/client.go -destination=clients/coingecko/mock/mock.go -package=mock_coingecko
	cd libs && mockgen -source=./clients/stripe/client.go -destination=clients/stripe/mock/mock.go -package=mock_stripe
	cd libs && mockgen -source=./backoff/retrypolicy/retrypolicy.go -destination=backoff/retrypolicy/mock/retrypolicy.go -package=mockretrypolicy
	cd libs && mockgen -source=./aws/s3.go -destination=aws/mock/mock.go -package=mockaws
	cd libs && mockgen -source=./kafka/dialer.go -destination=kafka/mock/dialer.go -package=mockdialer

instrumented:
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/grant -i Datastore -t ../.prom-gowrap.tmpl -o ./grant/instrumented_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/grant -i ReadOnlyDatastore -t ../.prom-gowrap.tmpl -o ./grant/instrumented_read_only_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/promotion -i Datastore -t ../.prom-gowrap.tmpl -o ./promotion/instrumented_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/promotion -i ReadOnlyDatastore -t ../.prom-gowrap.tmpl -o ./promotion/instrumented_read_only_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/skus -i Datastore -t ../.prom-gowrap.tmpl -o ./skus/instrumented_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/wallet -i Datastore -t ../.prom-gowrap.tmpl -o ./wallet/instrumented_datastore.go
	cd services && gowrap gen -p github.com/brave-intl/bat-go/services/wallet -i ReadOnlyDatastore -t ../.prom-gowrap.tmpl -o ./wallet/instrumented_read_only_datastore.go
	# fix everything called datastore...
	cd services && sed -i'bak' 's/datastore_duration_seconds/grant_datastore_duration_seconds/g' grant/instrumented_datastore.go
	cd services && sed -i'bak' 's/readonlydatastore_duration_seconds/grant_readonly_datastore_duration_seconds/g' ./grant/instrumented_read_only_datastore.go
	cd services && sed -i'bak' 's/datastore_duration_seconds/promotion_datastore_duration_seconds/g' ./promotion/instrumented_datastore.go
	cd services && sed -i'bak' 's/readonlydatastore_duration_seconds/promotion_readonly_datastore_duration_seconds/g' ./promotion/instrumented_read_only_datastore.go
	cd services && sed -i'bak' 's/datastore_duration_seconds/skus_datastore_duration_seconds/g' ./skus/instrumented_datastore.go
	cd services && sed -i'bak' 's/datastore_duration_seconds/wallet_datastore_duration_seconds/g' ./wallet/instrumented_datastore.go
	cd services && sed -i'bak' 's/readonlydatastore_duration_seconds/wallet_readonly_datastore_duration_seconds/g' ./wallet/instrumented_read_only_datastore.go
	# http clients
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/cbr -i Client -t ../.prom-gowrap.tmpl -o ./clients/cbr/instrumented_client.go
	sed -i'bak' 's/cbr.//g' libs/clients/cbr/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/ratios -i Client -t ../.prom-gowrap.tmpl -o ./clients/ratios/instrumented_client.go
	sed -i'bak' 's/ratios.//g' libs/clients/ratios/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/reputation -i Client -t ../.prom-gowrap.tmpl -o ./clients/reputation/instrumented_client.go
	sed -i'bak' 's/reputation.//g' libs/clients/reputation/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/gemini -i Client -t ../.prom-gowrap.tmpl -o ./clients/gemini/instrumented_client.go
	sed -i'bak' 's/gemini.//g' libs/clients/gemini/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/radom -i Client -t ../.prom-gowrap.tmpl -o ./clients/radom/instrumented_client.go
	sed -i'bak' 's/radom.//g' libs/clients/radom/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/bitflyer -i Client -t ../.prom-gowrap.tmpl -o ./clients/bitflyer/instrumented_client.go
	sed -i'bak' 's/bitflyer.//g' libs/clients/bitflyer/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/coingecko -i Client -t ../.prom-gowrap.tmpl -o ./clients/coingecko/instrumented_client.go
	sed -i'bak' 's/coingecko.//g' libs/clients/coingecko/instrumented_client.go
	cd libs && gowrap gen -p github.com/brave-intl/bat-go/libs/clients/stripe -i Client -t ../.prom-gowrap.tmpl -o ./clients/stripe/instrumented_client.go
	sed -i'bak' 's/stripe.//g' libs/clients/stripe/instrumented_client.go
	# fix all instrumented cause the interfaces are all called "client"
	sed -i'bak' 's/client_duration_seconds/cbr_client_duration_seconds/g' libs/clients/cbr/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/ratios_client_duration_seconds/g' libs/clients/ratios/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/reputation_client_duration_seconds/g' libs/clients/reputation/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/gemini_client_duration_seconds/g' libs/clients/gemini/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/bitflyer_client_duration_seconds/g' libs/clients/bitflyer/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/coingecko_client_duration_seconds/g' libs/clients/coingecko/instrumented_client.go
	sed -i'bak' 's/client_duration_seconds/stripe_client_duration_seconds/g' libs/clients/stripe/instrumented_client.go

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
	VAULT_TOKEN=$(VAULT_TOKEN) PKG=$(TEST_PKG) RUN=$(TEST_RUN) docker-compose -f docker-compose.yml -f docker-compose.dev.yml run --rm dev make test && cd main && go run main.go generate json-schema

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
	cp tools/settlement/config.hcl target/settlement-tools/
	cp tools/settlement/README.md target/settlement-tools/
	cp tools/settlement/hashicorp.asc target/settlement-tools/
	cd main/ && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -ldflags "-w -s -X main.version=${GIT_VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${GIT_COMMIT}" -o ../target/settlement-tools/bat-cli
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
	cd target/settlement-tools && gpg --import hashicorp.asc
	cd target/settlement-tools && gpg --verify vault_$(VAULT_VERSION)_SHA256SUMS.sig vault_$(VAULT_VERSION)_SHA256SUMS
	cd target/settlement-tools && grep $(GOOS)_$(GOARCH) vault_$(VAULT_VERSION)_SHA256SUMS | shasum -a 256 -c
	cd target/settlement-tools && unzip -o vault_$(VAULT_VERSION)_$(GOOS)_$(GOARCH).zip vault && rm vault_$(VAULT_VERSION)_*

vault:
	./target/settlement-tools/vault server -config=./target/settlement-tools/config.hcl

vault-clean:
	rm -rf share-0.gpg target/settlement-tools/vault-data vault-data

json-schema:
	cd main && go run main.go generate json-schema --overwrite

create-json-schema:
	cd main && go run main.go generate json-schema

test:
	cd libs && go test -count 1 -v -p 1 $(TEST_FLAGS) ./...
	cd services && go test -count 1 -v -p 1 $(TEST_FLAGS) ./...
	cd tools && go test -count 1 -v -p 1 $(TEST_FLAGS) ./...
	cd cmd && go test -count 1 -v -p 1 $(TEST_FLAGS) ./...

format:
	gofmt -s -w ./

format-lint:
	make format && make lint

lint: ensure-gomod-volume
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/main golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/cmd golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/libs golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/services golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/tools golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/serverless/email/webhook golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...
	docker run --rm -v "$$(pwd):/app" -v batgo_lint_gomod:/go/pkg --workdir /app/serverless/email/unsubscribe golangci/golangci-lint:v1.49.0 golangci-lint run -v ./...

download-mod:
	cd ./cmd && go mod download && cd ..
	cd ./libs && go mod download && cd ..
	cd ./main && go mod download && cd ..
	cd ./services && go mod download && cd ..
	cd ./tools && go mod download && cd ..
	cd ./serverless/email/status && go mod download && cd ../../..
	cd ./serverless/email/unsubscribe && go mod download && cd ../../..
	cd ./serverless/email/webhook && go mod download && cd ../../..

docker-up-ext: ensure-shared-net
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.dev.yml -f docker-compose.ext.yml run --rm -p 3333:3333 dev /bin/bash

ensure-gomod-volume:
	docker volume create batgo_lint_gomod

ensure-shared-net:
	if [ -z $$(docker network ls -q -f "name=brave_shared_net") ]; then docker network create brave_shared_net; fi
