# pass "go" and collect 200 BAT

[![Build
Status](https://travis-ci.org/brave-intl/bat-go.svg?branch=master)](https://travis-ci.org/brave-intl/bat-go)

## Developer Setup

1. [Install Go 1.12](https://golang.org/doc/install) (NOTE: Go 1.10 and earlier will not work!)

2. [Install GolangCI-Lint](https://github.com/golangci/golangci-lint#install)

3. Clone this repo via `git clone https://github.com/brave-intl/bat-go`

4. Build via `make`

**Consider adding a pre-commit hook**

1. Use your favorite editor to open `.git/hooks/pre-commit`
2. Add the following contents

   ```
   make test lint
   ```

3. Make the executable runnable by executing `chmod +x .git/hooks/pre-commit`
4. Commit away!

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

Once you are in the Docker container you can run the web server via `go run bin/grant-server/main.go`.

If you want to run tests you can do so via the command `go test --tags=integration -v`
For example in `promotion` you can run specific tests by running a command similar to `go test --tags=integration -run TestControllersTestSuite/TestCreateOrder`.

### Rapid Iteration dev Environment

On occasion it is desirable to re-run the development environment at will quickly.  To this
end you can run `make docker-refresh-dev` which will spin up the bat-go services including a
container named `grant-refresh-dev`.  If you want to recompile this service you merely need to
perform a `docker restart grant-refresh-dev` and it will recompile and run the service.

A particularly interesting use case is marrying this with utilities such as `fswatch` to watch
for file changes.  There is an example below which will restart this `grant-refresh-dev` container
on any file change in the source directory:

```bash
fswatch . | xargs -I {} sh -c '$(docker ps -f "name=grant-refresh-dev" --format "docker restart {{.ID}}")'
```

## Building a prod image using docker

You can build a docker image without installing the go toolchain. Ensure docker
is installed then run `make docker`.

