// Package pause provides the CLI endpoint to the "pause" command.
package pause

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/pkg/client"
)

// Command returns the "pause" command used to pause a managed torrent.
func Command() *cobra.Command {
	var address string

	cmd := &cobra.Command{
		Use:   "pause <hash>",
		Short: "Pause a managed torrent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(address)
			if err != nil {
				return err
			}

			if err = c.Pause(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("failed to pause torrent: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&address, "address", "a", "http://localhost:7373", "URL of the torrents server")

	return cmd
}
