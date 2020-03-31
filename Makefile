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

docker:
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t bat-go:latest .
	docker tag bat-go:latest bat-go:$(GIT_VERSION)

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

docker-cory-dev:
	$(eval VAULT_TOKEN = $(shell docker logs grant-vault 2>&1 | grep "Root Token" | tail -1 | cut -d ' ' -f 3 ))
	VAULT_TOKEN=$(VAULT_TOKEN) docker-compose -f docker-compose.yml -f docker-compose.cory.yml up -d cory

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
	GOOS=$(GOOS) GOARCH=$(GOARCH) make target/settlement-tools/paypal-settlement
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
