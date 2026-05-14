// Package add provides the CLI endpoint to the "add" command.
package add

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/pkg/client"
)

// Command returns the "add" command used to add a torrent by magnet URI.
func Command() *cobra.Command {
	var (
		address   string
		label     string
		targetDir string
	)

	cmd := &cobra.Command{
		Use:   "add <magnet>",
		Short: "Add a torrent by magnet URI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(address)
			if err != nil {
				return err
			}

			torrent, err := c.AddMagnet(cmd.Context(), args[0], label, targetDir)
			if err != nil {
				return fmt.Errorf("failed to add torrent: %w", err)
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(torrent)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&address, "address", "a", "http://localhost:7373", "URL of the torrents server")
	flags.StringVarP(&label, "label", "l", "", "optional human-readable label")
	flags.StringVarP(&targetDir, "target-dir", "d", "", "optional target download directory")

	return cmd
}
