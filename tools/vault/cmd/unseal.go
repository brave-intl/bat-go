package vault

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"

	rootcmd "github.com/brave-intl/bat-go/cmd"

	appctx "github.com/brave-intl/bat-go/libs/context"
	vaultsigner "github.com/brave-intl/bat-go/tools/vault/signer"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	// UnsealCmd initializes the vault server
	UnsealCmd = &cobra.Command{
		Use:   "unseal",
		Short: "unseals the vault server",
		Run:   rootcmd.Perform("unseal vault", Unseal),
	}
)

func init() {
	VaultCmd.AddCommand(
		UnsealCmd,
	)
}

// Unseal unseals the vault to allow for insertions
func Unseal(command *cobra.Command, args []string) error {
	logger, err := appctx.GetLogger(command.Context())
	if err != nil {
		return err
	}
	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	fi, err := os.Stdin.Stat()
	if err != nil {
		return err
	}

	var b []byte

	if (fi.Mode() & os.ModeNamedPipe) == 0 {
		fmt.Print("Please enter your unseal key: ")
		b, err = term.ReadPassword(int(os.Stdin.Fd()))
	} else {
		reader := bufio.NewReader(os.Stdin)
		b, err = ioutil.ReadAll(reader)
	}
	if err != nil {
		return err
	}

	status, err := wrappedClient.Client.Sys().Unseal(string(b))
	if err != nil {
		return err
	}

	t := status.T
	sealed := status.Sealed
	logEvent := logger.Info().
		Str("Seal Type", status.Type).
		Bool("Sealed", sealed).
		Int("Total Shares", status.N).
		Int("Threshold", t)

	if sealed {
		logEvent = logEvent.Int("progress", status.Progress).
			Int("total", t).
			Str("nonce", status.Nonce)
	}
	logEvent.Send()
	if err != nil {
		return err
	}
	return nil
}
