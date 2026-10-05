package templates

// ServicesView is the state of the server's units, timers and disks.
type ServicesView struct {
	Configured bool
	Units      []SvcUnit
	Down       int
	Timers     []SvcTimer
	Disks      []SvcDisk
	SmartAge   string // "SMART leído hace 3 h"
	Errors     []string
}

// SvcUnit is one systemd unit.
type SvcUnit struct {
	Name  string
	State string // "corriendo", "caído", …
	OK    bool
	Since string
}

// SvcTimer is one scheduled job.
type SvcTimer struct {
	Name   string
	Last   string
	Next   string
	Result string
	Failed bool
}

// SvcDisk is one disk's SMART reading.
type SvcDisk struct {
	Disk   string
	Model  string
	Asleep bool
	Health string
	Bad    bool
	Facts  []string // "34 °C", "1351 h", "desgaste 7 %", "0 reasignados"…
	Warn   []string // what is worth worrying about
}
