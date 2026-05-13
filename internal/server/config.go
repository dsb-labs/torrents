// Package server provides the torrents HTTP server and its configuration.
package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type (
	// The Config type contains the top-level configuration for the torrents server.
	Config struct {
		// HTTP server settings.
		HTTP HTTPConfig `toml:"http"`
		// On-disk state settings.
		Data DataConfig `toml:"data"`
		// Logging settings.
		Logging LoggingConfig `toml:"logging"`
	}

	// The HTTPConfig type contains configuration for the HTTP listener.
	HTTPConfig struct {
		// The address the HTTP server binds to, in host:port form.
		Address string `toml:"address"`
	}

	// The DataConfig type contains configuration for the server's on-disk state.
	DataConfig struct {
		// The directory under which the SQLite database, torrent metainfo, and
		// downloaded content are stored.
		Directory string `toml:"directory"`
	}

	// The LoggingConfig type contains configuration for application logging.
	LoggingConfig struct {
		// The minimum level to emit. One of "debug", "info", "warn", "error".
		Level string `toml:"level"`
	}
)

// DefaultConfig returns a Config populated with sensible defaults for running
// the torrents server out-of-box.
func DefaultConfig() Config {
	return Config{
		HTTP: HTTPConfig{
			Address: ":7373",
		},
		Data: DataConfig{
			Directory: defaultDataDir(),
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data"
	}

	return filepath.Join(home, ".local", "share", "torrents")
}

// LoadConfig the configuration file at the specified path. The configuration file is expected in TOML format.
func LoadConfig(path string) (Config, error) {
	var config Config
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return Config{}, fmt.Errorf("failed to decode config file: %w", err)
	}

	return config, nil
}

// Validate the configuration fields.
func (c *Config) Validate() error {
	return errors.Join(
		c.HTTP.validate(),
		c.Data.validate(),
		c.Logging.validate(),
	)
}

func (c HTTPConfig) validate() error {
	if c.Address == "" {
		return errors.New("http address is required")
	}

	return nil
}

func (c DataConfig) validate() error {
	if c.Directory == "" {
		return errors.New("data directory is required")
	}

	return nil
}

func (c LoggingConfig) validate() error {
	switch strings.ToLower(c.Level) {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid log level: %q", c.Level)
	}
}
