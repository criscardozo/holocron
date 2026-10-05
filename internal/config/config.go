// Package config loads server configuration from flags and environment
// variables. Flags take precedence over environment variables, which take
// precedence over built-in defaults.
package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the server startup configuration. Application-level settings
// (media paths, external service credentials, API keys) live in the database
// and are edited from the UI, not here.
type Config struct {
	// Addr is the listen address, e.g. ":8090". Binds all interfaces by
	// default because the dashboard is reached from other LAN machines.
	// Port 8090 rather than 8080: qBittorrent's WebUI owns 8080 on this host.
	Addr string
	// DBPath is the path to the SQLite database file.
	DBPath string
	// LogLevel is one of: debug, info, warn, error.
	LogLevel string

	// What the services screen watches. Configuration of the server, not of
	// the app: on Ginebra it is the same list ginebra-vigia uses, so the
	// screen and the alerting cannot disagree about what should be running.
	// Set in a systemd drop-in; empty means the screen has nothing to show.
	WatchUnits  []string // HOLOCRON_WATCH_UNITS, space-separated, e.g. "jellyfin caddy mnt-biblioteca.mount"
	WatchTimers string   // HOLOCRON_WATCH_TIMERS, a name prefix, e.g. "ginebra-"
	SmartFile   string   // HOLOCRON_SMART_FILE, the JSON a root timer writes

	// MachineName is what the screens call this computer ("Apagar Ginebra").
	// HOLOCRON_MACHINE_NAME, or the hostname with a capital letter.
	MachineName string
}

// Load parses flags and environment variables into a Config. It is meant to be
// called once at startup.
func Load() Config {
	c := Config{
		Addr:     envOr("HOLOCRON_ADDR", ":8090"),
		DBPath:   envOr("HOLOCRON_DB", defaultDBPath()),
		LogLevel: envOr("HOLOCRON_LOG_LEVEL", "info"),

		WatchUnits:  strings.Fields(os.Getenv("HOLOCRON_WATCH_UNITS")),
		WatchTimers: os.Getenv("HOLOCRON_WATCH_TIMERS"),
		SmartFile:   os.Getenv("HOLOCRON_SMART_FILE"),
		MachineName: envOr("HOLOCRON_MACHINE_NAME", hostTitle()),
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

// hostTitle is the short hostname with its first letter upper-cased: "ginebra"
// becomes "Ginebra". Empty when the hostname cannot be read.
func hostTitle() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return ""
	}
	h, _, _ = strings.Cut(h, ".")
	return strings.ToUpper(h[:1]) + h[1:]
}
