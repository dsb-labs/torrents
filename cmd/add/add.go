// Package add provides the CLI endpoint to the "add" command.
package add

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/pkg/client"
)

// Command returns the "add" command used to add a torrent by magnet URI
// or by uploading a .torrent metainfo file.
func Command() *cobra.Command {
	var address string
	var label string
	var targetDir string

	cmd := &cobra.Command{
		Use:   "add <magnet|file>",
		Short: "Add a torrent by magnet URI or .torrent file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(address)
			if err != nil {
				return err
			}

			var torrent client.Torrent
			if strings.HasPrefix(args[0], "magnet:?") {
				torrent, err = c.AddMagnet(cmd.Context(), args[0], label, targetDir)
			} else {
				var f *os.File
				f, err = os.Open(args[0])
				if err != nil {
					return fmt.Errorf("failed to open torrent file: %w", err)
				}
				defer f.Close()

				torrent, err = c.AddFile(cmd.Context(), f, label, targetDir)
			}
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
