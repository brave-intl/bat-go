# pass "go" and collect 200 BAT

[![Build
Status](https://travis-ci.org/brave-intl/bat-go.svg?branch=master)](https://travis-ci.org/brave-intl/bat-go)

## Building using docker

You can build a docker image without installing the go toolchain. Ensure docker
is installed then run `make docker`.

## Developer Setup

1. [Install Go 1.10](https://golang.org/doc/install) (NOTE: Go 1.9 and earlier will not work!)

2. Run `go get -d github.com/brave-intl/bat-go`.

3. [dep](https://github.com/golang/dep) is used to install the dependencies.  If you do not have dep, you need to [install it](https://github.com/golang/dep#setup). On mac:
`brew install dep`

4. `cd` into `~/go/github.com/brave-intl/bat-go`, then run `dep ensure` to install the dependencies

5. Build via `make`

6. Run the server executable `./grant-server`

## Creating Grants

1. Set the appropriate env vars: `GRANT_SIGNATOR_PRIVATE_KEY` and `GRANT_SIGNATOR_PUBLIC_KEY`

2. run the following command, adjusting the number of grants, expiry and maturity dates as needed
```
./create-tokens --env=true --num-grants=100 --expiry-date=2022-05-08T00:00:00-0000 --maturity-date=2018-10-01T00:00:00-0000
```

3. Run the following command to check your newly created tokens are tied to the correct key
```
./verify-tokens
```


