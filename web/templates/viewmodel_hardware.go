package templates

// HardwareView is one reading of the hardware screen, already formatted.
//
// Everything visual that depends on a number — a bar's length, a line's shape
// — arrives as SVG geometry rather than a CSS width. The CSP forbids inline
// styles, and an SVG attribute is not a style.
type HardwareView struct {
	// Live is false for a reading taken without a previous one, which has no
	// rates yet: the screen says "midiendo" instead of showing zeros.
	Live bool

	CPU      string // "23 %"
	CPUSpark Spark
	Cores    []HWCore
	Temp     string // "51 °C", or "" when unknown
	TempHot  bool   // above the point worth a colour
	TempSpk  Spark
	Load     string // "0,27 · 0,32 · 0,26"
	Uptime   string

	RAM     HWMeter
	Swap    HWMeter
	Zram    string // "3,8 MiB → 1,1 MiB (×3,3)", or ""
	HasSwap bool

	Links  []HWLink
	NetSpk Spark // received, the larger of the two on a media server
	Disks  []HWDisk

	Battery HWBattery
}

// Spark is a small line chart: the points of an SVG polyline on a 100×24
// canvas, plus the value at the top of the scale.
type Spark struct {
	Points string
	Max    string
}

// HWCore is one logical CPU.
type HWCore struct {
	Name  string // "0", "1"…
	Busy  string // "34 %"
	Width string // bar length on a 0..100 canvas
	MHz   string // "3,9 GHz"
	Hot   bool   // busy enough to colour
}

// HWMeter is a used-of-total bar.
type HWMeter struct {
	Used  string
	Total string
	Pct   string
	Width string
	High  bool
}

// HWLink is a network interface.
type HWLink struct {
	Name  string
	Up    bool
	Speed string // "1 Gb/s", or ""
	Rx    string
	Tx    string
}

// HWDisk is one disk's activity.
type HWDisk struct {
	Name  string
	Model string
	Read  string
	Write string
	Busy  string
	Width string
	Idle  bool
}

// HWBattery is the battery that, on Ginebra, is the UPS.
type HWBattery struct {
	Present     bool
	Percent     string
	Width       string
	Status      string // in Spanish
	Health      string // "65 %"
	Discharging bool   // mains is out
	Left        string // "1 h 52 min", or ""
	Low         bool
}
