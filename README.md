# pass "go" and collect 200 BAT

## Setup

1. Install Go https://golang.org/doc/install

2. Fork this repository 

3. In the directory `.../goworkspace/src/github.com/your-github-username`, run `go get github.com/your-github-username/bat-go`. At this point you will get an error because you do not have the correct dependencies:
```
# github.com/brave-intl/bat-go/grant
../../brave-intl/bat-go/grant/grant.go:77:9: undefined: jose.JSONWebKey
```
4. `dep` is used to install the depedencies.  If you do not have dep, you need to install it. On mac:
`brew install dep`

5. `cd` into bat-go, then run `dep ensure` to install the dependencies

6. Build the executable`go build`

7. Run the executable `./bat-go`

## TODO

* negative value checks
* add paranoid hard stops
  * enforce a hard-coded maximum grant allowance per-user
  * enforce a hard-coded maximum grant allowance per-redemption (of multiple
  * enforce a hard-coded maximum grant allowance per-redemption (of multiple
    grants)
