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

	"github.com/dsb-labs/torrents/internal/server/api"
	"github.com/dsb-labs/torrents/internal/server/database"
	"github.com/dsb-labs/torrents/internal/server/service"
	"github.com/dsb-labs/torrents/internal/server/torrent"
	"github.com/dsb-labs/torrents/internal/server/ui"
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

	pieces := database.NewPieceRepository(db)
	if err = pieces.Load(ctx); err != nil {
		return fmt.Errorf("failed to load piece completion cache: %w", err)
	}

	client, err := torrent.NewClient(logger, filepath.Join(config.Data.Directory, "downloads"), pieces)
	if err != nil {
		return fmt.Errorf("failed to start torrent client: %w", err)
	}
	defer client.Close()

	engine := torrent.New(torrent.Config{
		Logger: logger,
		Client: client,
	})

	torrents := service.NewTorrentService(logger, engine, database.NewTorrentRepository(db), pieces)

	if err = torrents.Restore(ctx); err != nil {
		return fmt.Errorf("failed to restore torrents: %w", err)
	}

	mux := http.NewServeMux()
	api.NewTorrentAPI(torrents).Register(mux)
	ui.NewTorrentHandler(torrents).Register(mux)
	mux.Handle("GET /static/", ui.Static())

	var handler http.Handler = mux
	middlewares := []func(http.Handler) http.Handler{
		api.Recovery(logger),
		api.Logging(logger),
	}
	for _, m := range middlewares {
		handler = m(handler)
	}

	server := &http.Server{
		Addr:              config.HTTP.Address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(server.ListenAndServe)
	g.Go(func() error {
		<-ctx.Done()

		logger.Debug("shutting down http server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	})

	err = g.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
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
