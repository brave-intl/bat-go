// main - main entry-point to bat-go commands through cobra
// individual commands are outlined in ./cmd/
package main

import (
	"github.com/brave-intl/bat-go/cmd"
	// pull in rewards module. setup code is in init
	_ "github.com/brave-intl/bat-go/cmd/rewards"
	// pull in settlement module. setup code is in init
	_ "github.com/brave-intl/bat-go/cmd/settlement"
	// pull in vault module. setup code is in init
	_ "github.com/brave-intl/bat-go/cmd/vault"
	// pull in wallets module. setup code is in init
	_ "github.com/brave-intl/bat-go/cmd/wallets"
)

func main() {
	cmd.Execute()
}
