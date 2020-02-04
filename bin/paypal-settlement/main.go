package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/settlement/paypal"
	"github.com/shopspring/decimal"
)

var (
	input    = flag.String("in", "", "the file or comma delimited list of files that should be utilized")
	currency = flag.String("currency", "", "a currency must be set")
	auth     = os.Getenv("RATE_AUTH")
	rate     = flag.Float64("rate", 0, "the rate to compute the currency conversion")
	out      = flag.String("out", "./paypal-settlement", "the location of the file")
)

func main() {
	var err error
	flag.Parse()
	command := flag.Arg(0)
	switch command {
	case "transform":
		err = paypal.CreateSettlementFile(paypal.TransformArgs{
			In:       *input,
			Currency: *currency,
			Auth:     auth,
			Rate:     decimal.NewFromFloat(*rate),
			Out:      *out,
		})
	case "complete":
		err = paypal.CompleteSettlement(paypal.CompleteArgs{
			In:  *input,
			Out: *out,
		})
	case "upload":
		// upload()
	case "verify":
		// verify()
	default:
		err = errors.New("a command must be passed (transform, complete)")
	}
	if err != nil {
		flag.Usage()
		fmt.Println("ERROR:", err)
	}
}
