package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/barloc/dbt-esync/app/run"
)

// NewCommand returns the root cobra command.
func NewCommand() *cobra.Command {
	var config string

	cmd := &cobra.Command{
		Use:               Name,
		Short:             "Sync dbt exposures from BI tools and code repos into your dbt project",
		Version:           "v0.0.0",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := run.Run(config); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&config, "config", "c", "config.yaml", "Config file path")
	return cmd
}
