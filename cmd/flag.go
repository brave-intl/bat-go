package cmd

import (
	"fmt"

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

// GetStringSlice gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetStringSlice(key string) ([]string, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetStringSlice(key)
	if len(value) == 0 {
		return viper.GetStringSlice(key), nil
	}
	return value, err
}

// String attaches a string flag to the command
func (fb *FlagBuilder) String(key string, defaultValue string, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().String(key, defaultValue, description)
		})
}

// GetString gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetString(key string) (string, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetString(key)
	if len(value) == 0 {
		return viper.GetString(key), nil
	}
	return value, err
}

// Uint attaches an uint flag to the command
func (fb *FlagBuilder) Uint(key string, defaultValue uint, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Uint(key, defaultValue, description)
		})
}

// GetUint gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetUint(key string) (uint, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetUint(key)
	if value == 0 {
		return viper.GetUint(key), nil
	}
	return value, err
}

// Int attaches an int flag to the command
func (fb *FlagBuilder) Int(key string, defaultValue int, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Int(key, defaultValue, description)
		})
}

// GetInt gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetInt(key string) (int, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetInt(key)
	if value == 0 {
		return viper.GetInt(key), nil
	}
	return value, err
}

// Float64 attaches a float64 type flag to the command
func (fb *FlagBuilder) Float64(key string, defaultValue float64, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Float64(key, defaultValue, description)
		})
}

// GetFloat64 gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetFloat64(key string) (float64, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetFloat64(key)
	if value == 0 {
		return viper.GetFloat64(key), nil
	}
	return value, err
}

// Bool attaches a bool flag to the command
func (fb *FlagBuilder) Bool(key string, defaultValue bool, description string) *FlagBuilder {
	return fb.SetKey(key).
		loopCommands(func(command *cobra.Command) {
			command.Flags().Bool(key, defaultValue, description)
		})
}

// GetBool gets the flag value, defaulting back to envs
func (fb *FlagBuilder) GetBool(key string) (bool, error) {
	command := fb.commands[0]
	value, err := command.Flags().GetBool(key)
	if !value {
		return viper.GetBool(key), nil
	}
	return value, err
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
