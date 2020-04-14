// main - main entry-point to bat-go commands through cobra
// individual commands are outlined in ./cmd/
package main

import (
	"github.com/brave-intl/bat-go/cmd"
)

func main() {
	cmd.Execute()
}
