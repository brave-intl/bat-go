package consumers

import (
	"context"
	"encoding/json"
	"io/ioutil"

	// needed for profiling
	_ "net/http/pprof"
	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/cmd/wallets"
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// EyeshadeProducersCmd start up the eyeshade server
	EyeshadeProducersCmd = &cobra.Command{
		Use:   "eyeshade",
		Short: "subcommand to start eyeshade consumers",
		Run:   cmd.Perform("eyeshade", RunEyeshadeProducersCmd),
	}
)

func init() {
	ProducersCmd.AddCommand(EyeshadeProducersCmd)
	eyeshadeProducersFlags := cmd.NewFlagBuilder(EyeshadeProducersCmd)
	eyeshadeProducersFlags.Flag().String("input", "",
		"the file that should be produced from").
		Env("INPUT").
		Bind("input")
	eyeshadeProducersFlags.Flag().String("topic", "",
		"the topic or topic key to produce to").
		Env("TOPIC").
		Bind("topic")
}

// WithService creates a service
func WithService(
	ctx context.Context,
) (*eyeshade.Service, error) {
	return eyeshade.SetupService(
		eyeshade.WithContext(ctx),
		eyeshade.WithNewLogger,
		eyeshade.WithBuildInfo,
		eyeshade.WithProducer(avro.AllTopics...),
		eyeshade.WithTopicAutoCreation,
	)
}

// RunEyeshadeProducersCmd is the runner for starting up the eyeshade server
func RunEyeshadeProducersCmd(cmd *cobra.Command, args []string) error {
	service, err := WithService(
		cmd.Context(),
	)
	if err == nil {
		return nil
	}
	file := viper.GetViper().GetString("input")
	switch viper.GetViper().GetString("topic") {
	case avro.TopicKeys.Contribution, avro.KeyToTopic[avro.TopicKeys.Contribution]:
		var t []models.Contribution
		err := Parse(file, &t)
		if err != nil {
			return err
		}
		return service.ProduceContributions(cmd.Context(), t)
	case avro.TopicKeys.Referral, avro.KeyToTopic[avro.TopicKeys.Referral]:
		var t []models.Referral
		err := Parse(file, &t)
		if err != nil {
			return err
		}
		return service.ProduceReferrals(cmd.Context(), t)
	case avro.TopicKeys.Settlement, avro.KeyToTopic[avro.TopicKeys.Settlement]:
		var t []models.Settlement
		err := Parse(file, &t)
		if err != nil {
			return err
		}
		return service.ProduceSettlements(cmd.Context(), t)
	case avro.TopicKeys.Suggestion, avro.KeyToTopic[avro.TopicKeys.Suggestion]:
		var t []models.Suggestion
		err := Parse(file, &t)
		if err != nil {
			return err
		}
		return service.ProduceSuggestions(cmd.Context(), t)
	}
	return nil
}

// Parse parses a given file
func Parse(file string, p interface{}) error {
	statement, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(statement, p)
}
