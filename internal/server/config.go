// Package server provides the torrents HTTP server and its configuration.
package server

import (
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

type (
	// The Config type contains the top-level configuration for the torrents server.
	Config struct {
		// HTTP server settings.
		HTTP HTTPConfig `toml:"http"`
		// Logging settings.
		Logging LoggingConfig `toml:"logging"`
	}

	// The HTTPConfig type contains configuration for the HTTP listener.
	HTTPConfig struct {
		// The address the HTTP server binds to, in host:port form.
		Address string `toml:"address"`
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
		Logging: LoggingConfig{
			Level: "info",
		},
	}
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
		c.Logging.validate(),
	)
}

func (c HTTPConfig) validate() error {
	if c.Address == "" {
		return errors.New("http address is required")
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
