# pass "go" and collect 200 BAT

[![Build
Status](https://travis-ci.org/brave-intl/bat-go.svg?branch=master)](https://travis-ci.org/brave-intl/bat-go)

## Developer Setup

1. [Install Go 1.12](https://golang.org/doc/install) (NOTE: Go 1.10 and earlier will not work!)

2. Clone this repo via `git clone https://github.com/brave-intl/bat-go`

3. Build via `make`

## Full environment via docker-compose

Ensure docker and docker-compose are installed.

Ensure that your `.env` file is populated with values for each of the
env vars that does not have a default in `docker-compose.yml`.

### Local prod-like environment

To bring up a prod-like environment, run `docker-compose up -d`.

### Local dev environment

To bring up a dev environment, run `make docker-dev`.

This brings up an additional vault service, used for integration testing of
some auxiliary binaries.

You can run the unit and integration tests via `make docker-test`

Once you are in the Docker containe you can run the web server via `go run bin/grant-server/main.go`.

If you want to run tests you can do so via the command `go test --tags=integration -v`
For example in `promotion` you can run specific tests by running a command similar to `go test --tags=integration -run TestControllersTestSuite/TestCreateOrder`.


## Building a prod image using docker

You can build a docker image without installing the go toolchain. Ensure docker
is installed then run `make docker`.
