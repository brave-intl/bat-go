package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/jsonschema"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/outputs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var overwrite bool

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(jsonSchemaCmd)

	// overwrite - defaults to false
	jsonSchemaCmd.PersistentFlags().BoolVarP(&overwrite, "overwrite", "", false,
		"overwrite the existing json schema files")
	must(viper.BindPFlag("overwrite", jsonSchemaCmd.PersistentFlags().Lookup("overwrite")))
	must(viper.BindEnv("overwrite", "OVERWRITE"))
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "entrypoint to generate subcommands",
}

var jsonSchemaCmd = &cobra.Command{
	Use:   "json-schema",
	Short: "entrypoint to generate json schema for project",
	Run:   jsonSchemaRun,
}

// jsonSchemaRun - main entrypoint for the `generate json-schema` subcommand
func jsonSchemaRun(cmd *cobra.Command, args []string) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}
	logger.Info().Msg("starting json-schema generation")

	// Wallet Outputs ./wallet/outputs.go
	for _, t := range outputs.APIResponseTypes {

		logger.Info().Str("path", t.PkgPath()).Str("name", t.Name()).Str("str", t.String()).Msg("type being processed")

		schema, err := jsonschema.ReflectFromType(t).MarshalJSON()
		if err != nil {
			logger.Error().Err(err).Msg("failed to generate json schema")
			<-time.After(1 * time.Second)
			os.Exit(1)
		}

		parts := strings.Split(t.String(), ".")

		// read old schema file
		existingSchema, err := ioutil.ReadFile(
			fmt.Sprintf("./schema/%s/%s", parts[0], parts[1]))
		if err != nil {
			logger.Info().Err(err).Msg("could not find existing schema file, might be a new api")
		} else {
			// test equality of schema file with what we just generated
			if !bytes.Equal(existingSchema, schema) {
				if viper.GetBool("overwrite") {
					logger.Warn().Msg(fmt.Sprintf("Schema has changed: %s.%s", parts[0], parts[1]))
				} else {
					logger.Error().Msg(fmt.Sprintf("Schema has changed: %s.%s", parts[0], parts[1]))
					<-time.After(1 * time.Second)
					os.Exit(1)
				}
			}
		}

		err = ioutil.WriteFile(
			fmt.Sprintf("./schema/%s/%s", parts[0], parts[1]),
			schema, 0644)
		if err != nil {
			logger.Error().Err(err).Msg("failed to generate json schema")
			<-time.After(1 * time.Second)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "%s\n", schema)
	}

	logger.Info().Msg("completed json-schema generation")
	<-time.After(1 * time.Second)
}
