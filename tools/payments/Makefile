.PHONY: build clean tidy
.DEFAULT: build

build: dist/vault dist/prepare dist/authorize dist/configure dist/bootstrap dist/validate dist/status dist/chain-account dist/disaster
tidy: tidy-vault tidy-prepare tidy-authorize tidy-configure tidy-bootstrap tidy-validate tidy-status tidy-chain-account tidy-disaster
clean: clean-vault clean-prepare clean-authorize clean-configure clean-bootstrap clean-validate clean-status clean-chain-account clean-disaster

dist/vault:
	cd cmd/vault && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/vault main.go
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
dist/disaster:
	cd cmd/disaster && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/disaster main.go
dist/report:
	cd cmd/report && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o ../../dist/report main.go

tidy-vault:
	cd cmd/vault && go mod tidy
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
tidy-disaster:
	cd cmd/disaster && go mod tidy
tidy-report:
	cd cmd/report && go mod tidy

clean-vault:
	cd cmd/vault && go clean
	rm -f dist/vault
clean-prepare:
	cd cmd/prepare && go clean
	rm -f dist/prepare
clean-authorize:
	cd cmd/authorize && go clean
	rm -f dist/authorize
clean-configure:
	cd cmd/configure && go clean
	rm -f dist/configure
clean-bootstrap:
	cd cmd/bootstrap && go clean
	rm -f dist/bootstrap
clean-validate:
	cd cmd/validate && go clean
	rm -f dist/validate
clean-status:
	cd cmd/status && go clean
	rm -f dist/status
clean-chain-account:
	cd cmd/chain-account && go clean
	rm -f dist/chain-account
clean-disaster:
	cd cmd/disaster && go clean
	rm -f dist/disaster
clean-report:
	cd cmd/report && go clean
	rm -f dist/report
