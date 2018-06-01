GIT_VERSION := $(shell git describe --abbrev=8 --dirty --always --tags)
VAULT_VERSION=0.10.1
_BINS := $(wildcard bin/*)
ifdef GOOS
	BINS := $(_BINS:bin/%=target/$(GOOS)_$(GOARCH)/%)
else
	BINS := $(_BINS:bin/%=%)
endif

.PHONY: all bins docker test lint clean
all: test bins
	
bins: clean $(BINS)

.DEFAULT:
	go build ./bin/$@

target/%:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ ./bin/$(notdir $@) 

docker:
	docker build -t bat-go:latest .
	docker tag bat-go:latest bat-go:$(GIT_VERSION)

mac:
	GOOS=darwin GOARCH=amd64 make bins

settlement-tools:
	mkdir -p target/settlement-tools
	cp settlement/config.hcl target/settlement-tools/
	cp settlement/README.md target/settlement-tools/
	GOOS=darwin GOARCH=amd64 make target/settlement-tools/vault-init
	GOOS=darwin GOARCH=amd64 make target/settlement-tools/vault-unseal
	GOOS=darwin GOARCH=amd64 make target/settlement-tools/vault-import-key
	GOOS=darwin GOARCH=amd64 make target/settlement-tools/vault-create-wallet
	GOOS=darwin GOARCH=amd64 make target/settlement-tools/vault-sign-settlement
	GOOS=darwin GOARCH=amd64 make download-vault

download-vault:
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_$(GOOS)_$(GOARCH).zip
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_SHA256SUMS
	cd target/settlement-tools && curl -Os https://releases.hashicorp.com/vault/$(VAULT_VERSION)/vault_$(VAULT_VERSION)_SHA256SUMS.sig
	cd target/settlement-tools && gpg --verify vault_$(VAULT_VERSION)_SHA256SUMS.sig vault_$(VAULT_VERSION)_SHA256SUMS
	cd target/settlement-tools && grep $(GOOS)_$(GOARCH) vault_$(VAULT_VERSION)_SHA256SUMS | shasum -a 256 -c 
	cd target/settlement-tools && unzip -o vault_$(VAULT_VERSION)_$(GOOS)_$(GOARCH).zip vault && rm vault_$(VAULT_VERSION)_*

test:
	go test -v --tags=$(TEST_TAGS) ./...

lint:
	gometalinter --vendor --disable=gocyclo --enable=misspell --deadline=5m ./...

clean:
	rm -f $(BINS)
