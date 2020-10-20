package macaroon

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
)

var (
	// MacaroonCmd is a subcommand for macaroons
	MacaroonCmd = &cobra.Command{
		Use:   "macaroon",
		Short: "macaroon subcommand",
	}
	// MacaroonGenerateCmd generates a macaroon
	MacaroonGenerateCmd = &cobra.Command{
		Use:   "gen",
		Short: "generate a macaroon",
		Run:   cmd.Perform("macaroon generation", RunMacaroonGenerate),
	}
)

// RunMacaroonGenerate runs the generate command
func RunMacaroonGenerate(cmd *cobra.Command, args []string) error {
	config, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	secret, err := cmd.Flags().GetString("secret")
	if err != nil {
		return err
	}
	return Generate(
		cmd.Context(),
		config,
		secret,
	)
}

// Generate generates macaroons
func Generate(ctx context.Context, config, secret string) error {
	// new config
	logger, lerr := appctx.GetLogger(ctx)
	if lerr != nil {
		_, logger = logging.SetupLogger(ctx)
	}

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
			Interface("token", t).
			Msg("token")
	}
	return nil
}
