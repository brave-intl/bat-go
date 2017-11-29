# pass "go" and collect 200 BAT

## Developer Setup

1. [Install Go](https://golang.org/doc/install)

2. Run `go get -d github.com/brave-intl/bat-go`. 

Note that if you try to build at this point you will get an error because you do not have the correct dependencies:
```
# github.com/brave-intl/bat-go/grant
../../brave-intl/bat-go/grant/grant.go:77:9: undefined: jose.JSONWebKey
```
3. [dep](https://github.com/golang/dep) is used to install the depedencies.  If you do not have dep, you need to [install it](https://github.com/golang/dep#setup). On mac:
`brew install dep`

4. `cd` into `~/go/github.com/brave-intl/bat-go`, then run `dep ensure` to install the dependencies

5. You can run the included tests via `go test ./...`

5. Build the executable`go build`

6. Run the executable `./bat-go`

## TODO

* negative value checks
* add paranoid hard stops
  * enforce a hard-coded maximum grant allowance per-user
  * enforce a hard-coded maximum grant allowance per-redemption (of multiple
  * enforce a hard-coded maximum grant allowance per-redemption (of multiple
    grants)
