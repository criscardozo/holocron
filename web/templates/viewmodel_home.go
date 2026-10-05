package templates

// HomeView is the start page: the machine at a glance, one tile per area, and
// what arrived lately.
type HomeView struct {
	Machine string
	// Status is the row of readings under the title: up for how long, CPU,
	// temperature, battery, traffic.
	Status []HomeStat
	Attn   []AttnChip
	Tiles  []Tile
	Recent []ActRecent
}

// HomeStat is one reading in the status row.
type HomeStat struct {
	Text string
	Tone string // "ok", "warn", "danger" or ""
}

// Tile is one area on the start page. It is the way in, so it says the one
// number that tells whether going in is worth it.
type Tile struct {
	Href  string
	Icon  string // sprite symbol id
	Tone  string // icon colour: pink, violet, teal, lilac, indigo, amber, sky, mint
	Title string
	Value string
	Sub   string
	// Warn marks something worth a look, Off an area with nothing configured.
	Warn, Off bool
}
