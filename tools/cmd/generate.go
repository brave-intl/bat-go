package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/jsonschema"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	cmdutils.RootCmd.AddCommand(GenerateCmd)
	GenerateCmd.AddCommand(JSONSchemaCmd)

	// overwrite - defaults to false
	JSONSchemaCmd.Flags().Bool("overwrite", false,
		"overwrite the existing json schema files")
	cmdutils.Must(viper.BindPFlag("overwrite", JSONSchemaCmd.Flags().Lookup("overwrite")))
}

// GenerateCmd is the generate command
var GenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "entrypoint to generate subcommands",
}

// JSONSchemaCmd is the json schema command
var JSONSchemaCmd = &cobra.Command{
	Use:   "json-schema",
	Short: "entrypoint to generate json schema for project",
	Run:   cmdutils.Perform("generate json schema", jsonSchemaRun),
}

// jsonSchemaRun - main entrypoint for the `generate json-schema` subcommand
func jsonSchemaRun(command *cobra.Command, args []string) error {
	ctx := command.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}
	overwrite, err := command.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}
	logger.Info().Msg("starting json-schema generation")

	// Wallet Outputs ./wallet/outputs.go
	for _, t := range APIResponseTypes {

		logger.Info().Str("path", t.PkgPath()).Str("name", t.Name()).Str("str", t.String()).Msg("type being processed")

		schema, err := jsonschema.ReflectFromType(t).MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to generate json schema: %w", err)
		}

		parts := strings.Split(t.String(), ".")

		// read old schema file
		existingSchema, err := ioutil.ReadFile(
			fmt.Sprintf("../schema/%s/%s", parts[0], parts[1]))
		if err != nil {
			logger.Info().Err(err).Msg("could not find existing schema file, might be a new api")
		} else {
			// test equality of schema file with what we just generated
			if !bytes.Equal(existingSchema, schema) {
				if overwrite {
					logger.Warn().Msg(fmt.Sprintf("Schema has changed: %s.%s", parts[0], parts[1]))
				} else {
					return fmt.Errorf("schema has changed: %s.%s", parts[0], parts[1])
				}
			}
		}

		if overwrite {
			err = ioutil.WriteFile(
				fmt.Sprintf("../schema/%s/%s", parts[0], parts[1]),
				schema, 0644)
			if err != nil {
				return fmt.Errorf("failed to generate json schema: %w", err)
			}
		}

		fmt.Fprintf(os.Stdout, "%s\n", schema)
	}

	logger.Info().Msg("completed json-schema generation")
	return nil
}
