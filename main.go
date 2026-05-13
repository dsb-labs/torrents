// Package main provides the entrypoint to the torrents binary.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/spf13/cobra"
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

	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
