package macaroon

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/cmd"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// MacaroonCmd is a subcommand for macaroons
	MacaroonCmd = &cobra.Command{
		Use:   "macaroon",
		Short: "macaroon subcommand",
	}
	// MacaroonCreateCmd generates a macaroon
	MacaroonCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "create a macaroon",
		Run:   cmd.Perform("macaroon creation", RunMacaroonCreate),
	}
)

func init() {
	MacaroonCmd.AddCommand(
		MacaroonCreateCmd,
	)
	cmd.RootCmd.AddCommand(
		MacaroonCmd,
	)

	createBuilder := cmdutils.NewFlagBuilder(MacaroonCreateCmd)

	createBuilder.Flag().String("config", "example.yaml",
		"the location of the config file").
		Bind("config")

	createBuilder.Flag().String("secret", "mysecret",
		"the value of the secret").
		Env("MACAROON_SECRET").
		Bind("secret")
}

// RunMacaroonCreate runs the generate command
func RunMacaroonCreate(command *cobra.Command, args []string) error {
	config, err := command.Flags().GetString("config")
	if err != nil {
		return err
	}
	secret := viper.GetString("secret")
	if secret == "" {
		secret, err = command.Flags().GetString("secret")
		if err != nil {
			return err
		}
	}
	return Generate(
		command.Context(),
		config,
		secret,
	)
}

// Generate generates macaroons
func Generate(ctx context.Context, config, secret string) error {
	logger, lerr := appctx.GetLogger(ctx)
	if lerr != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	// new config
	var tc = new(TokenConfig)
	// parse config file
	if err := tc.Parse(config); err != nil {
		return fmt.Errorf("unable to parse token config: %v", err)
	}

	for _, token := range tc.Tokens {
		// generate token
		t, err := token.Generate(secret)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("unable to generate token")
			continue
		}

		logger.Info().
			Str("id", token.ID).
			Int("secret_length", len(secret)).
			Interface("token", t).
			Msg("token")
	}
	return nil
}
