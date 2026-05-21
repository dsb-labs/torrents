// Package delete provides the CLI endpoint to the "delete" command.
package delete

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/pkg/client"
)

// Command returns the "delete" command used to delete a managed torrent.
func Command() *cobra.Command {
	var address string
	var deleteFiles bool

	cmd := &cobra.Command{
		Use:     "delete <hash>",
		Aliases: []string{"rm"},
		Short:   "Delete a managed torrent",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(address)
			if err != nil {
				return err
			}

			if err = c.Remove(cmd.Context(), args[0], deleteFiles); err != nil {
				return fmt.Errorf("failed to delete torrent: %w", err)
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&address, "address", "a", "http://localhost:7373", "URL of the torrents server")
	flags.BoolVar(&deleteFiles, "delete-files", false, "delete files")

	return cmd
}
