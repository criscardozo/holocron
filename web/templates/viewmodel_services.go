package templates

// ServicesView is the state of the server's units, timers and disks.
type ServicesView struct {
	Configured bool       `json:"configured"`
	Units      []SvcUnit  `json:"units"`
	Down       int        `json:"down"`
	Timers     []SvcTimer `json:"timers"`
	Disks      []SvcDisk  `json:"disks"`
	SmartAge   string     `json:"smartAge"` // "SMART leído hace 3 h"
	// Drift is nil on a server that does not check its own installation.
	Drift  *SvcDrift `json:"drift"`
	Errors []string  `json:"errors"`
}

// SvcDrift is the server's check of what it installed against its repo.
type SvcDrift struct {
	Age    string      `json:"age"`    // "revisado hace 20 min"
	Commit string      `json:"commit"` // the repo commit it compared against
	Stale  bool        `json:"stale"`  // the check itself has stopped running
	Differ []SvcDiffer `json:"differ"`
}

// SvcDiffer is one file that does not match.
type SvcDiffer struct {
	File    string `json:"file"`
	Problem string `json:"problem"`
}

// SvcUnit is one systemd unit.
type SvcUnit struct {
	Name  string `json:"name"`
	State string `json:"state"` // "corriendo", "caído", …
	OK    bool   `json:"ok"`
	Since string `json:"since"`
}

// SvcTimer is one scheduled job.
type SvcTimer struct {
	Name   string `json:"name"`
	Last   string `json:"last"`
	Next   string `json:"next"`
	Result string `json:"result"`
	Failed bool   `json:"failed"`
}

// SvcDisk is one disk's SMART reading.
type SvcDisk struct {
	Disk   string   `json:"disk"`
	Model  string   `json:"model"`
	Asleep bool     `json:"asleep"`
	Health string   `json:"health"`
	Bad    bool     `json:"bad"`
	Facts  []string `json:"facts"` // "34 °C", "1351 h", "desgaste 7 %", "0 reasignados"…
	Warn   []string `json:"warn"`  // what is worth worrying about
}
