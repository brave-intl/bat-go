package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/alecthomas/jsonschema"
	eyeshadeoutput "github.com/brave-intl/bat-go/eyeshade/output"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/outputs"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(GenerateCmd)
	GenerateCmd.AddCommand(JSONSchemaCmd)
	GenerateCmd.AddCommand(EyeshadeJSONSchemaCmd)

	// overwrite - defaults to false
	builder := NewFlagBuilder(JSONSchemaCmd).
		AddCommand(EyeshadeJSONSchemaCmd)

	builder.Flag().Bool("overwrite", false,
		"overwrite the existing json schema files")
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
	Run:   Perform("generate eyeshade json schema", jsonSchemaRun),
}

// EyeshadeJSONSchemaCmd is the json schema command
var EyeshadeJSONSchemaCmd = &cobra.Command{
	Use:   "eyeshade-json-schema",
	Short: "entrypoint to generate json schema for eyeshade",
	Run:   Perform("generate json schema", eyeshadeJSONSchemaRun),
}

func eyeshadeJSONSchemaRun(command *cobra.Command, args []string) error {
	overwrite, err := command.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}
	return jsonSchemaGenerate(
		command.Context(),
		overwrite,
		eyeshadeoutput.APIResponseTypes,
		map[string]string{
			"models":    "eyeshade",
			"countries": "eyeshade",
		},
	)
}

func jsonSchemaRun(command *cobra.Command, args []string) error {
	overwrite, err := command.Flags().GetBool("overwrite")
	if err != nil {
		return err
	}
	return jsonSchemaGenerate(command.Context(), overwrite, outputs.APIResponseTypes)
}

// jsonSchemaGenerate - main entrypoint for the `generate json-schema` subcommand
func jsonSchemaGenerate(
	ctx context.Context,
	overwrite bool,
	responseTypes []reflect.Type,
	transformers ...map[string]string,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}
	logger.Info().Msg("starting json-schema generation")

	// Wallet Outputs ./wallet/outputs.go
	for _, t := range responseTypes {

		logger.Info().Str("path", t.PkgPath()).Str("name", t.Name()).Str("str", t.String()).Msg("type being processed")

		schema, err := jsonschema.ReflectFromType(t).MarshalJSON()
		if err != nil {
			return fmt.Errorf("failed to generate json schema: %w", err)
		}

		str := t.String()
		for _, transformer := range transformers {
			for key, val := range transformer {
				str = strings.ReplaceAll(str, key, val)
			}
		}
		parts := strings.Split(str, ".")

		// read old schema file
		existingSchema, err := ioutil.ReadFile(
			fmt.Sprintf("./schema/%s/%s", parts[0], parts[1]))
		if err != nil && err != os.ErrNotExist {
			logger.Info().Err(err).Msg("could not find existing schema file, might be a new api")
		} else {
			// test equality of schema file with what we just generated
			if !bytes.Equal(existingSchema, schema) {
				if overwrite {
					logger.Warn().Str("module", parts[0]).Str("struct", parts[1]).Msg("schema has changed")
				} else {
					return fmt.Errorf("schema has changed: %s.%s", parts[0], parts[1])
				}
			}
		}

		err = ioutil.WriteFile(
			fmt.Sprintf("./schema/%s/%s", parts[0], parts[1]),
			schema, 0644)
		if err != nil {
			return fmt.Errorf("failed to generate json schema: %w", err)
		}

		fmt.Fprintf(os.Stdout, "%s\n", schema)
	}

	logger.Info().Msg("completed json-schema generation")
	return nil
}
