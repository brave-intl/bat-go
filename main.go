// main - main entry-point to bat-go commands through cobra
// individual commands are outlined in ./cmd/
package main

import (
	// unified tools/services binary
	"github.com/brave-intl/bat-go/cmd"
	// pull in tool module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/cmd"
	// pull in settlement module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/settlement/cmd"
	// pull in vault module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/vault/cmd"
	// pull in wallet module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/wallet/cmd"
	// pull in macaroon module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/macaroon/cmd"
	// pull in merchat module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/merchant/cmd"

	// pull in rewards module. setup code is in init
	_ "github.com/brave-intl/bat-go/services/rewards/cmd"
	// pull in wallets module. setup code is in init
	_ "github.com/brave-intl/bat-go/services/wallet/cmd"
	// pull in serve module. setup code is in init
	_ "github.com/brave-intl/bat-go/services/cmd"
	// pull in ratios module. setup code is in init
	_ "github.com/brave-intl/bat-go/services/ratios/cmd"
	// pull in grants module. setup code is in init
	_ "github.com/brave-intl/bat-go/services/grant/cmd"
)

var (
	// variables will be overwritten at build time
	version   string
	commit    string
	buildTime string
)

func main() {
	cmd.Execute(version, commit, buildTime)
}
