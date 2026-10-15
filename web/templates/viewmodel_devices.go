package templates

// DevicesView drives the Dispositivos page.
type DevicesView struct {
	Configured bool
	// File is the devices file's name, for the messages that send someone
	// to edit it.
	File string
	// Missing, ErrorLine and Failed say why there is nothing to show: no
	// file yet, a mistake on a given line, or anything else.
	Missing   bool
	ErrorLine int
	Failed    bool
	Groups    []DeviceGroup
}

// DeviceGroup is one heading and its devices. Plain groups (Otros) show as a
// list rather than cards.
type DeviceGroup struct {
	Name    string
	Plain   bool
	Devices []DeviceCard
}

// DeviceCard is one device. The photo URLs are empty when there is none.
type DeviceCard struct {
	Name, Model, IP, MAC string
	CityURL, DeviceURL   string
}
