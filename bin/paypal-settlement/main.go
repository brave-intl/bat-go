package main

import (
	"flag"
	"fmt"

	"github.com/brave-intl/bat-go/bin/paypal-settlement/data"
	"github.com/brave-intl/bat-go/bin/paypal-settlement/transform"
	"github.com/shopspring/decimal"
)

var input = flag.String("in", "", "the file or comma delimited list of files that should be utilized")
var currency = flag.String("currency", "", "a currency must be set")
var auth = flag.String("auth", "", "an authorization bearer token must be set")
var date = flag.String("date", "", "the date of the batch")
var rate = flag.Float64("rate", 0, "the rate to compute the currency conversion")
var out = flag.String("out", "./paypal-payout.csv", "the location of the file")

func main() {
	var err error
	flag.Parse()
	command := flag.Arg(0)
	args := data.Args{
		In:       *input,
		Currency: *currency,
		Auth:     *auth,
		Date:     *date,
		Rate:     decimal.NewFromFloat(*rate),
		Out:      *out,
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
