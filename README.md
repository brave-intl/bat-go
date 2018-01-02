# pass "go" and collect 200 BAT

[![Build
Status](https://travis-ci.org/brave-intl/bat-go.svg?branch=master)](https://travis-ci.org/brave-intl/bat-go)

## Developer Setup

1. [Install Go 1.9](https://golang.org/doc/install)

2. Run `go get -d github.com/brave-intl/bat-go`. 

3. [dep](https://github.com/golang/dep) is used to install the dependencies.  If you do not have dep, you need to [install it](https://github.com/golang/dep#setup). On mac:
`brew install dep`

4. `cd` into `~/go/github.com/brave-intl/bat-go`, then run `dep ensure` to install the dependencies

5. Build via `make`

6. Run the server executable `./grant-server`
