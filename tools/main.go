// main - main entry-point to bat-go commands through cobra
// individual commands are outlined in ./cmd/
package main

import (
	toolscmd "github.com/brave-intl/bat-go/tools/cmd"

	// pull in settlement module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/settlement/cmd"
	// pull in vault module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/vault/cmd"
	// pull in macaroon module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/macaroon/cmd"
	// pull in merchat module. setup code is in init
	_ "github.com/brave-intl/bat-go/tools/merchant/cmd"
)

var (
	// variables will be overwritten at build time
	version   string
	commit    string
	buildTime string
)

func main() {
	toolscmd.Execute(version, commit, buildTime)
}
