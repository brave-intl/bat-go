package cmd

import (
	"context"
	"crypto/tls"
	"errors"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/pindialer"
	"github.com/spf13/cobra"
)

var (
	// GetCertFingerprintCmd is the GetCertFingerprint command
	GetCertFingerprintCmd = &cobra.Command{
		Use:   "get-cert-fingerprint",
		Short: "A helper for fetching tls fingerprint info for pinning",
		Run:   rootcmd.Perform("get cert fingerprint", GetCertFingerprint),
	}
)

func init() {
	rootcmd.RootCmd.AddCommand(GetCertFingerprintCmd)
}

// GetCertFingerprint runs the command for GetCertFingerprint
func GetCertFingerprint(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("no arguments detected")
	}
	return CheckFingerprints(cmd.Context(), args)
}

// CheckFingerprints checks the fingerprints at the following address
func CheckFingerprints(ctx context.Context, addresses []string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	for _, address := range addresses {
		logger.Info().
			Str("address", address).
			Msg("dialing")
		c, err := tls.Dial("tcp", address, nil)
		if err != nil {
			return err
		}
		prints, err := pindialer.GetFingerprints(c)
		if err != nil {
			return err
		}
		for key, value := range prints {
			logger.Info().
				Str("issuer", key).
				Str("fingerprint", value).
				Msg("issuer fingerprint")
		}
	}
	return nil
}
