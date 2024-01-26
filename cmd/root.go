package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/migrate-tool/pkg/config"
)

func New(use, short string) *cobra.Command {
	var configFilePath string
	config := &config.Config{}

	command := &cobra.Command{
		Use:               use,
		Short:             short,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Usage()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			content, err := os.ReadFile(configFilePath)
			if err != nil {
				return fmt.Errorf("unable to read config, err: %w", err)
			}

			return json.Unmarshal(content, config)
		},
	}

	// Global flags
	command.PersistentFlags().StringVarP(&configFilePath, "config", "c", "config.json", "Path to the config file")

	// Child commands
	command.AddCommand(newDumpCommand(config))
	command.AddCommand(newPatchCommand(config))
	command.AddCommand(newUpdateCommand(config))

	return command
}
