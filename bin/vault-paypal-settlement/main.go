package main

import (
	"flag"
	"fmt"

	"github.com/brave-intl/bat-go/bin/vault-paypal-settlement/data"
	"github.com/brave-intl/bat-go/bin/vault-paypal-settlement/transform"
)

var input = flag.String("in", "", "the file that should be utilized")
var currency = flag.String("currency", "", "a currency must be set")
var auth = flag.String("auth", "", "an authorization bearer token must be set")

func main() {
	var err error
	flag.Parse()
	command := flag.Arg(0)
	args := data.Args{
		In:       *input,
		Currency: *currency,
		Auth:     *auth,
	}
	switch command {
	case "transform":
		err = transform.Input(args)
	case "upload":
		// upload()
	case "verify":
		// verify()
	}
	if err != nil {
		flag.Usage()
		fmt.Println("ERROR:", err)
	}
}
