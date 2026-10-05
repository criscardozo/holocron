package templates

// HardwareView is one reading of the hardware screen, already formatted.
//
// Everything visual that depends on a number — a bar's length, a line's shape
// — arrives as SVG geometry rather than a CSS width. The CSP forbids inline
// styles, and an SVG attribute is not a style.
type HardwareView struct {
	// Live is false for a reading taken without a previous one, which has no
	// rates yet: the screen says "midiendo" instead of showing zeros.
	Live bool `json:"live"`

	CPU      string   `json:"cpu"` // "23 %"
	CPUSpark Spark    `json:"cpuSpark"`
	Cores    []HWCore `json:"cores"`
	Temp     string   `json:"temp"`    // "51 °C", or "" when unknown
	TempHot  bool     `json:"tempHot"` // above the point worth a colour
	TempSpk  Spark    `json:"tempSpk"`
	Load     string   `json:"load"` // "0,27 · 0,32 · 0,26"
	Uptime   string   `json:"uptime"`

	RAM     HWMeter `json:"ram"`
	Swap    HWMeter `json:"swap"`
	Zram    string  `json:"zram"` // "3,8 MiB → 1,1 MiB (×3,3)", or ""
	HasSwap bool    `json:"hasSwap"`

	Links  []HWLink `json:"links"`
	NetSpk Spark    `json:"netSpk"` // received, the larger of the two on a media server
	Disks  []HWDisk `json:"disks"`

	Battery HWBattery `json:"battery"`
}

// Spark is a small line chart: the points of an SVG polyline on a 100×24
// canvas, plus the value at the top of the scale.
type Spark struct {
	Points string `json:"points"`
	Max    string `json:"max"`
}

// HWCore is one logical CPU.
type HWCore struct {
	Name  string `json:"name"`  // "0", "1"…
	Busy  string `json:"busy"`  // "34 %"
	Width string `json:"width"` // bar length on a 0..100 canvas
	MHz   string `json:"mhz"`   // "3,9 GHz"
	Hot   bool   `json:"hot"`   // busy enough to colour
}

// HWMeter is a used-of-total bar.
type HWMeter struct {
	Used  string `json:"used"`
	Total string `json:"total"`
	Pct   string `json:"pct"`
	Width string `json:"width"`
	High  bool   `json:"high"`
}

// HWLink is a network interface.
type HWLink struct {
	Name  string `json:"name"`
	Up    bool   `json:"up"`
	Speed string `json:"speed"` // "1 Gb/s", or ""
	Rx    string `json:"rx"`
	Tx    string `json:"tx"`
}

// HWDisk is one disk's activity.
type HWDisk struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Read  string `json:"read"`
	Write string `json:"write"`
	Busy  string `json:"busy"`
	Width string `json:"width"`
	Idle  bool   `json:"idle"`
}

// HWBattery is the battery that, on Ginebra, is the UPS.
type HWBattery struct {
	Present     bool   `json:"present"`
	Percent     string `json:"percent"`
	Width       string `json:"width"`
	Status      string `json:"status"`      // in Spanish
	Health      string `json:"health"`      // "65 %"
	Discharging bool   `json:"discharging"` // mains is out
	Left        string `json:"left"`        // "1 h 52 min", or ""
	Low         bool   `json:"low"`
}
