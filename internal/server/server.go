package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dsb-labs/torrents/internal/server/database"
	"github.com/dsb-labs/torrents/internal/server/service"
	"github.com/dsb-labs/torrents/internal/server/torrent"
)

// Run starts the torrents server using the given configuration and blocks
// until the context is cancelled or the server stops with an error.
func Run(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logger := newLogger(config.Logging)
	logger.With("address", config.HTTP.Address).Debug("starting torrents server")

	if err := os.MkdirAll(config.Data.Directory, 0o755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	db, err := database.Open(ctx, database.Config{
		Logger: logger,
		Path:   filepath.Join(config.Data.Directory, "state.db"),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	client, err := torrent.NewClient(filepath.Join(config.Data.Directory, "downloads"))
	if err != nil {
		return fmt.Errorf("failed to start torrent client: %w", err)
	}
	defer client.Close()

	engine := torrent.New(torrent.Config{
		Logger: logger,
		Client: client,
	})

	torrents := service.NewTorrentService(logger, engine, database.NewTorrentRepository(db))

	if err := torrents.Restore(ctx); err != nil {
		return fmt.Errorf("failed to restore torrents: %w", err)
	}

	mux := http.NewServeMux()
	_ = torrents

	server := &http.Server{
		Addr:              config.HTTP.Address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server failed: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()

		logger.Debug("shutting down http server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shut down http server: %w", err)
		}

		return nil
	})

	return g.Wait()
}

func newLogger(config LoggingConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(config.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
