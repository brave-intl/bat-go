# pass "go" and collect 200 BAT

## Developer Setup

1. [Install Go](https://golang.org/doc/install)

2. Run `go get -d github.com/brave-intl/bat-go`. 

3. [dep](https://github.com/golang/dep) is used to install the depedencies.  If you do not have dep, you need to [install it](https://github.com/golang/dep#setup). On mac:
`brew install dep`

4. `cd` into `~/go/github.com/brave-intl/bat-go`, then run `dep ensure` to install the dependencies

5. You can run the included tests via `go test ./...`

5. Build the executable `go build`

6. Run the executable `./bat-go`
