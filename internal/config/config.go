// Package config loads server configuration from flags and environment
// variables. Flags take precedence over environment variables, which take
// precedence over built-in defaults.
package config

import (
	"flag"
	"fmt"
	"github.com/cristian/holocron/internal/version"
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
	// UpdatesFile is HOLOCRON_UPDATES_FILE, the JSON the server's update
	// script writes for the Gestión page. Empty hides the panel.
	UpdatesFile string
	// Disks is HOLOCRON_DISKS: the server's disks, "name:mount:role[:scan]"
	// separated by ";". Scan narrows the size scan to one folder of the disk
	// (on the system disk, the downloads, not the whole system).
	Disks    []DiskSpec
	BadDisks []string
	// QualitySkipSubs is HOLOCRON_QUALITY_SKIP_SUBS: Jellyfin library names,
	// separated by ";", left out of the quality panel's subtitle categories.
	// On Ginebra, Anime;Museum, whose titles get their subtitles on the way
	// through Películas or Series.
	QualitySkipSubs []string
	// DevicesDir is HOLOCRON_DEVICES_DIR: the directory holding
	// dispositivos.toml and its photos, for the Dispositivos page. On the
	// server, never in the repository: it lists the house's addresses.
	DevicesDir string
	// DirectHost is HOLOCRON_DIRECT_HOST, the server's address on the home
	// network for the Stack page's direct links. Found on its own when unset.
	DirectHost string

	// MachineName is what the screens call this computer ("Apagar Ginebra").
	// HOLOCRON_MACHINE_NAME, or the hostname with a capital letter.
	MachineName string

	// MediaFolders, when set, is the watched-folder list and the settings form
	// stops editing it. HOLOCRON_MEDIA_FOLDERS, entries separated by ";",
	// each "label:purpose:path" with purpose disk, movies or tv:
	//   Películas:movies:/mnt/biblioteca/Peliculas;Disco4:disk:/mnt/disco4
	MediaFolders []FolderSpec
	// BadFolders are entries of HOLOCRON_MEDIA_FOLDERS that could not be read,
	// for the log.
	BadFolders []string
}

// FolderSpec is one entry of HOLOCRON_MEDIA_FOLDERS.
type FolderSpec struct{ Label, Purpose, Path string }

// parseFolders reads HOLOCRON_MEDIA_FOLDERS. A malformed entry is skipped
// rather than fatal, and reported by the caller as a count mismatch would
// hide it: so it is returned in bad.
func parseFolders(v string) (specs []FolderSpec, bad []string) {
	for _, e := range strings.Split(v, ";") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		parts := strings.SplitN(e, ":", 3)
		if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
			bad = append(bad, e)
			continue
		}
		specs = append(specs, FolderSpec{
			Label: strings.TrimSpace(parts[0]), Purpose: strings.TrimSpace(parts[1]), Path: strings.TrimSpace(parts[2]),
		})
	}
	return specs, bad
}

// splitList reads a ";"-separated list, dropping blanks.
func splitList(v string) []string {
	var out []string
	for _, e := range strings.Split(v, ";") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
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
		UpdatesFile: os.Getenv("HOLOCRON_UPDATES_FILE"),
		DirectHost:  os.Getenv("HOLOCRON_DIRECT_HOST"),
		DevicesDir:  os.Getenv("HOLOCRON_DEVICES_DIR"),
		MachineName: envOr("HOLOCRON_MACHINE_NAME", hostTitle()),
	}
	c.MediaFolders, c.BadFolders = parseFolders(os.Getenv("HOLOCRON_MEDIA_FOLDERS"))
	c.Disks, c.BadDisks = parseDisks(os.Getenv("HOLOCRON_DISKS"))
	c.QualitySkipSubs = splitList(os.Getenv("HOLOCRON_QUALITY_SKIP_SUBS"))

	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address (host:port)")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "path to the SQLite database file")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log level: debug, info, warn, error")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	// Both before anything is opened: run by hand to check the version, the
	// binary used to start a server instead, and on the way create a database
	// in the home of whoever ran it. A stray argument ("holocron version") is
	// refused for the same reason rather than ignored.
	if *showVersion {
		fmt.Println(version.Current())
		os.Exit(0)
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "holocron: unexpected argument %q (flags only; -version prints the version)\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}

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

// DiskSpec is one disk from HOLOCRON_DISKS.
type DiskSpec struct {
	Name, Mount, Role, Scan string
}

// parseDisks reads "name:mount:role[:scan]" entries separated by ";". Scan
// defaults to the mount. Malformed entries are returned apart, to be logged.
func parseDisks(raw string) (specs []DiskSpec, bad []string) {
	for _, e := range strings.Split(raw, ";") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		parts := strings.SplitN(e, ":", 4)
		if len(parts) < 3 || parts[0] == "" || !strings.HasPrefix(parts[1], "/") {
			bad = append(bad, e)
			continue
		}
		d := DiskSpec{
			Name:  strings.TrimSpace(parts[0]),
			Mount: strings.TrimSpace(parts[1]),
			Role:  strings.TrimSpace(parts[2]),
			Scan:  strings.TrimSpace(parts[1]),
		}
		if len(parts) == 4 && strings.HasPrefix(strings.TrimSpace(parts[3]), "/") {
			d.Scan = strings.TrimSpace(parts[3])
		}
		specs = append(specs, d)
	}
	return specs, bad
}
