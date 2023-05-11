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
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -ldflags "-w -s -X main.version=${GIT_VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${GIT_COMMIT}" -o bat-go-tools main.go

docker:
	docker rmi -f bat-go-tools:latest
	docker build --build-arg COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(GIT_VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) -t bat-go-tools:$(GIT_VERSION)$(BUILD_TIME) .
	docker tag bat-go:$(GIT_VERSION)$(BUILD_TIME) bat-go-tools:latest

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

test:
	GODEBUG=x509ignoreCN=0 go test -count 1 -v -p 1 $(TEST_FLAGS)

format:
	gofmt -s -w ./

format-lint:
	make format && make lint
lint:
	golangci-lint run -E gofmt -E revive --exclude-use-default=false
