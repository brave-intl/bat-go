package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	id int
)

// FlagBuilder creates a flag builder
type FlagBuilder struct {
	id       int
	commands []*cobra.Command
	key      string
}

func init() {
	viper.AutomaticEnv()
}

// Bind runs the BindPFlag function
func (fb *FlagBuilder) Bind(key string) *FlagBuilder {
	return fb.loopCommands(func(command *cobra.Command) {
		Must(viper.BindPFlag(key, command.Flags().Lookup(key)))
	})
}

// SetKey sets the key to be shared across methods
func (fb *FlagBuilder) SetKey(key string) *FlagBuilder {
	if fb.key != "" {
		Must(fmt.Errorf("key has already been set to '%s' cannot set to '%s' try calling .Flag() before starting to define a new flag", fb.key, key))
	}
	fb.key = key
	return fb
}

// Flag resets the builder to allow for chaining
func (fb *FlagBuilder) Flag() *FlagBuilder {
	fb.key = ""
	return fb
}

// StringSlice attaches a string slice flag to the command
func (fb *FlagBuilder) StringSlice(key string, defaultValue []string, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().StringSlice(key, defaultValue, description)
		})
}

// String attaches a string flag to the command
func (fb *FlagBuilder) String(key string, defaultValue string, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().String(key, defaultValue, description)
		})
}

// BoolP attaches a boolean flag to the command
func (fb *FlagBuilder) BoolP(key string, shortKey string, defaultValue bool, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().BoolP(key, shortKey, defaultValue, description)
		})
}

// Duration attaches a string flag to the command
func (fb *FlagBuilder) Duration(key string, defaultValue time.Duration, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Duration(key, defaultValue, description)
		})
}

// Uint attaches an uint flag to the command
func (fb *FlagBuilder) Uint(key string, defaultValue uint, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Uint(key, defaultValue, description)
		})
}

// Int attaches an int flag to the command
func (fb *FlagBuilder) Int(key string, defaultValue int, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Int(key, defaultValue, description)
		})
}

// Float64 attaches a float64 type flag to the command
func (fb *FlagBuilder) Float64(key string, defaultValue float64, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Float64(key, defaultValue, description)
		})
}

// Bool attaches a bool flag to the command
func (fb *FlagBuilder) Bool(key string, defaultValue bool, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Bool(key, defaultValue, description)
		})
}

// Require requires the flag
func (fb *FlagBuilder) Require() *FlagBuilder {
	return fb.loopCommands(func(command *cobra.Command) {
		Must(command.MarkFlagRequired(fb.key))
	})
}

// Env attaches an env
func (fb *FlagBuilder) Env(env string) *FlagBuilder {
	Must(viper.BindEnv(fb.key, env))
	return fb
}

// NewFlagBuilder creates a new FlagBuilder from one command
func NewFlagBuilder(command *cobra.Command) *FlagBuilder {
	id++
	commands := []*cobra.Command{}
	fb := FlagBuilder{id, commands, ""}
	if command != nil {
		fb.AddCommand(command)
	}
	return &fb
}

// AddCommand adds a command
func (fb *FlagBuilder) AddCommand(command *cobra.Command) *FlagBuilder {
	fb.commands = append(fb.commands, command)
	return fb
}

// Concat combine flag builders
func (fb *FlagBuilder) Concat(builders ...*FlagBuilder) *FlagBuilder {
	newBuilder := NewFlagBuilder(nil)
	allBuilders := append([]*FlagBuilder{fb}, builders...)
	for _, builder := range allBuilders {
		newBuilder.commands = append(newBuilder.commands, builder.commands...)
	}
	return newBuilder
}

func (fb *FlagBuilder) loopCommands(iterator func(*cobra.Command)) *FlagBuilder {
	for _, command := range fb.commands {
		iterator(command)
	}
	return fb
}

// Must helper to make sure there is no errors
func Must(err error) {
	if err != nil {
		log.Printf("failed to initialize: %s\n", err.Error())
		// exit with failure
		os.Exit(1)
	}
}
