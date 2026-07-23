// Package config loads server configuration from flags and environment
// variables. Flags take precedence over environment variables, which take
// precedence over built-in defaults.
package config

import (
	"flag"
	"os"
	"path/filepath"
)

// Config holds the server startup configuration. Application-level settings
// (media paths, external service credentials, API keys) live in the database
// and are edited from the UI, not here.
type Config struct {
	// Addr is the listen address, e.g. ":8080". Binds all interfaces by
	// default because the dashboard is reached from other LAN machines.
	Addr string
	// DBPath is the path to the SQLite database file.
	DBPath string
	// LogLevel is one of: debug, info, warn, error.
	LogLevel string
}

// Load parses flags and environment variables into a Config. It is meant to be
// called once at startup.
func Load() Config {
	c := Config{
		Addr:     envOr("HOLOCRON_ADDR", ":8080"),
		DBPath:   envOr("HOLOCRON_DB", defaultDBPath()),
		LogLevel: envOr("HOLOCRON_LOG_LEVEL", "info"),
	}

	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address (host:port)")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "path to the SQLite database file")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log level: debug, info, warn, error")
	flag.Parse()

	return c
}

// defaultDBPath returns the default database location under the user's data
// directory, falling back to the working directory if the home dir is unknown.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "holocron.db"
	}
	return filepath.Join(home, ".local", "share", "holocron", "holocron.db")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
