// main - main entry-point to bat-go commands through cobra
// individual commands are outlined in ./cmd/
package main

import (
	// pull in tool module. setup code is in init

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/payments-service/cmd"

	// pull in payments service
	_ "github.com/brave-intl/payments-service/services/nitro"
	_ "github.com/brave-intl/payments-service/services/payments/cmd"
)

var (
	// variables will be overwritten at build time
	version   string
	commit    string
	buildTime string
)

func main() {
	defer func() {
		if logging.Writer != nil {
			logging.Writer.Close()
		}
	}()
	cmd.Execute(version, commit, buildTime)
}
