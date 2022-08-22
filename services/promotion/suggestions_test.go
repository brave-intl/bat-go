package promotion

import (
	"testing"
	"time"

	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/stretchr/testify/assert"
)

func TestDeduplicateCredentialBindings(t *testing.T) {

	var tokens = []CredentialBinding{
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_1",
		},
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_2",
		},
	}
	var seen = []CredentialBinding{}

	var result = DeduplicateCredentialBindings(tokens...)
	if len(result) > len(tokens) {
		t.Error("result should be less than number of tokens")
	}

	for _, v := range result {
		for _, vv := range seen {
			if v == vv {
				t.Error("Deduplication of tokens didn't work")
			}
			seen = append(seen, v)
		}
	}
}

func TestUnmarshalText(t *testing.T) {
	encoded := "eyJ0eXBlIjogImF1dG8tY29udHJpYnV0ZSIsICJjaGFubmVsIjogImJyYXZlLmNvbSJ9"
	var expected, d Suggestion
	expected.Type = "auto-contribute"
	expected.Channel = "brave.com"

	err := d.Base64Decode(encoded)
	assert.NoError(t, err, "Failed to unmarshal")
	assert.Equal(t, expected, d)
}

func TestTryUpgradeSuggestionEvent(t *testing.T) {
	var (
		service Service
		err     error
	)

	service.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"suggestion": suggestionEventSchema,
	})

	assert.NoError(t, err, "Failed to initialize codecs")

	suggestion := `{"id":"d6e6f7f2-8975-4105-8fef-2ad89e299add","type":"oneoff-tip","channel":"3zsistemi.si","totalAmount":"10","funding":[{"type":"ugp","amount":"10","cohort":"control","promotion":"1d54793b-e8e7-4e96-890f-a1836cab9533"}]}`

	upgraded, err := service.TryUpgradeSuggestionEvent([]byte(suggestion))
	assert.NoError(t, err, "Failed to upgrade suggestion event")

	native, _, err := service.codecs["suggestion"].NativeFromBinary(upgraded)
	assert.NoError(t, err)

	createdAt, err := time.Parse(time.RFC3339, native.(map[string]interface{})["createdAt"].(string))
	assert.NoError(t, err)

	assert.True(t, (time.Since(createdAt)) < time.Second)

	suggestionBytes := []byte{72, 100, 54, 101, 54, 102, 55, 102, 50, 45, 56, 57, 55, 53, 45, 52, 49, 48, 53, 45, 56, 102, 101, 102, 45, 50, 97, 100, 56, 57, 101, 50, 57, 57, 97, 100, 100, 20, 111, 110, 101, 111, 102, 102, 45, 116, 105, 112, 24, 51, 122, 115, 105, 115, 116, 101, 109, 105, 46, 115, 105, 60, 50, 48, 49, 57, 45, 49, 49, 45, 49, 53, 84, 50, 51, 58, 50, 57, 58, 49, 49, 46, 49, 52, 49, 52, 48, 53, 56, 55, 53, 90, 4, 49, 48, 2, 6, 117, 103, 112, 4, 49, 48, 14, 99, 111, 110, 116, 114, 111, 108, 72, 49, 100, 53, 52, 55, 57, 51, 98, 45, 101, 56, 101, 55, 45, 52, 101, 57, 54, 45, 56, 57, 48, 102, 45, 97, 49, 56, 51, 54, 99, 97, 98, 57, 53, 51, 51, 0}

	upgraded, err = service.TryUpgradeSuggestionEvent(suggestionBytes)
	assert.NoError(t, err, "Failed to upgrade suggestion event")

	assert.Equal(t, suggestionBytes, upgraded)
}
