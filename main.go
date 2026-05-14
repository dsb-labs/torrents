// Package main provides the entrypoint to the torrents binary.
//
//go:generate go tool mockery
//go:generate go tool templ generate
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/dsb-labs/torrents/cmd/add"
	delcmd "github.com/dsb-labs/torrents/cmd/delete"
	"github.com/dsb-labs/torrents/cmd/list"
	"github.com/dsb-labs/torrents/cmd/pause"
	"github.com/dsb-labs/torrents/cmd/resume"
	"github.com/dsb-labs/torrents/cmd/serve"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()

	cmd := &cobra.Command{
		Use:          "torrents",
		Short:        "A self-hostable torrent client and manager",
		SilenceUsage: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		cmd.Version = info.Main.Version
	}

	cmd.AddCommand(
		serve.Command(),
		add.Command(),
		list.Command(),
		delcmd.Command(),
		pause.Command(),
		resume.Command(),
	)

	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
