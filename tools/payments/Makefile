.PHONY: build clean tidy
.DEFAULT: build

build: dist/create-vault dist/prepare dist/authorize dist/configure dist/bootstrap dist/validate dist/status dist/chain-account
tidy: tidy-create tidy-prepare tidy-authorize tidy-configure tidy-bootstrap tidy-validate tidy-status tidy-chain-account
clean: clean-create clean-prepare clean-authorize clean-configure clean-bootstrap clean-validate clean-status clean-chain-account

dist/create-vault:
	cd cmd/create && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/create-vault main.go
dist/prepare:
	cd cmd/prepare && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/prepare main.go
dist/authorize:
	cd cmd/authorize && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/authorize main.go
dist/configure:
	cd cmd/configure && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/configure main.go
dist/bootstrap:
	cd cmd/bootstrap && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/bootstrap main.go
dist/validate:
	cd cmd/validate && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/validate main.go
dist/status:
	cd cmd/status && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/status main.go
dist/chain-account:
	cd cmd/chain-account && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/chain-account main.go

tidy-create:
	cd cmd/create && go mod tidy
tidy-prepare:
	cd cmd/prepare && go mod tidy
tidy-authorize:
	cd cmd/authorize && go mod tidy
tidy-configure:
	cd cmd/configure && go mod tidy
tidy-bootstrap:
	cd cmd/bootstrap && go mod tidy
tidy-validate:
	cd cmd/validate && go mod tidy
tidy-status:
	cd cmd/status && go mod tidy
tidy-chain-account:
	cd cmd/chain-account && go mod tidy

clean-create:
	cd cmd/create && go clean
	rm dist/create-vault
clean-prepare:
	cd cmd/prepare && go clean
	rm dist/prepare
clean-authorize:
	cd cmd/authorize && go clean
	rm dist/authorize
clean-configure:
	cd cmd/configure && go clean
	rm dist/configure
clean-bootstrap:
	cd cmd/bootstrap && go clean
	rm dist/bootstrap
clean-validate:
	cd cmd/validate && go clean
	rm dist/validate
clean-status:
	cd cmd/status && go clean
	rm dist/status
clean-chain-account:
	cd cmd/chain-account && go clean
	rm dist/chain-account
