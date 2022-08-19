package settlement

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/spf13/cobra"
)

func init() {
	rootcmd.RootCmd.AddCommand(SettlementCmd)
}

// SettlementCmd is the settlement command
var SettlementCmd = &cobra.Command{
	Use:   "settlement",
	Short: "provides settlement utilities",
}

// WriteCategorizedTransactions write out transactions categorized under a key
func WriteCategorizedTransactions(
	ctx context.Context,
	outPath string,
	transactions map[string][]custodian.Transaction,
) error {
	for key, txs := range transactions {
		if len(txs) > 0 {
			outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
			err := WriteTransactions(ctx, outputPath, txs)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteTransactions writes settlement transactions to a json file
func WriteTransactions(ctx context.Context, outPath string, metadata []custodian.Transaction) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if len(metadata) == 0 {
		return nil
	}

	logger.Debug().Str("files", outPath).Int("num transactions", len(metadata)).Msg("writing outputting files")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		logger.Error().Err(err).Msg("failed writing outputting files")
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}
