// Package serve provides the CLI endpoint to the "serve" command.
package serve

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/internal/server"
)

// Command returns the "serve" command used to start and run the torrents server.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "serve [config-file]",
		Short: "Run the torrents server",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config := server.DefaultConfig()

			if len(args) > 0 {
				var err error
				config, err = server.LoadConfig(args[0])
				if err != nil {
					return fmt.Errorf("failed to load configuration file: %w", err)
				}
			}

			return server.Run(cmd.Context(), config)
		},
	}
}
