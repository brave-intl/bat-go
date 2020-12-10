package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	id int = 0
)

// FlagBuilder creates a flag builder
type FlagBuilder struct {
	id       int
	commands []*cobra.Command
	key      string
}

// Bind runs the BindPFlag function
func (fb *FlagBuilder) Bind(key string) *FlagBuilder {
	fb.loopCommands(func(command *cobra.Command) {
		Must(viper.BindPFlag(key, command.Flags().Lookup(key)))
	})
	return fb
}

// String attaches a string flag to the command
func (fb *FlagBuilder) String(key string, defaultValue string, description string) *FlagBuilder {
	fb.key = key
	fb.loopCommands(func(command *cobra.Command) {
		command.Flags().String(key, defaultValue, description)
	})
	return fb
}

// Int attaches an int flag to the command
func (fb *FlagBuilder) Int(key string, defaultValue int, description string) *FlagBuilder {
	fb.key = key
	fb.loopCommands(func(command *cobra.Command) {
		command.Flags().Int(key, defaultValue, description)
	})
	return fb
}

// Float64 attaches a float64 type flag to the command
func (fb *FlagBuilder) Float64(key string, defaultValue float64, description string) *FlagBuilder {
	fb.key = key
	fb.loopCommands(func(command *cobra.Command) {
		command.Flags().Float64(key, defaultValue, description)
	})
	return fb
}

// Bool attaches a bool flag to the command
func (fb *FlagBuilder) Bool(key string, defaultValue bool, description string) *FlagBuilder {
	fb.key = key
	fb.loopCommands(func(command *cobra.Command) {
		command.Flags().Bool(key, defaultValue, description)
	})
	return fb
}

// Require requires the flag
func (fb *FlagBuilder) Require() *FlagBuilder {
	fb.loopCommands(func(command *cobra.Command) {
		Must(command.MarkFlagRequired(fb.key))
	})
	return fb
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

func (fb *FlagBuilder) loopCommands(iterator func(*cobra.Command)) {
	for _, command := range fb.commands {
		iterator(command)
	}
}
