package templates

// HomeView is the start page: the machine at a glance, one tile per area, and
// what arrived lately.
type HomeView struct {
	Machine string `json:"machine"`
	// Status is the row of readings under the title: up for how long, CPU,
	// temperature, battery, traffic.
	Status []HomeStat  `json:"status"`
	Attn   []AttnChip  `json:"attention"`
	Tiles  []Tile      `json:"tiles"`
	Recent []ActRecent `json:"recent"`
	// Apps are the doors to the rest of the stack, in the order they are used.
	Apps []AppLink `json:"apps"`
	// Mural is filled in for the app only: the web draws its wall from the
	// request context, in the layout.
	Mural []string `json:"mural,omitempty"`
}

// HomeStat is one reading in the status row.
type HomeStat struct {
	Text string `json:"text"`
	Tone string `json:"tone"` // "ok", "warn", "danger" or ""
}

// Tile is one area on the start page. It is the way in, so it says the one
// number that tells whether going in is worth it.
type Tile struct {
	Href  string `json:"href"` // the web page; the app maps it to its screen
	Icon  string `json:"icon"` // sprite symbol id
	Tone  string `json:"tone"` // icon colour: pink, violet, teal, lilac, indigo, amber, sky, mint
	Title string `json:"title"`
	Value string `json:"value"`
	Sub   string `json:"sub"`
	// Warn marks something worth a look, Off an area with nothing configured.
	Warn bool `json:"warn"`
	Off  bool `json:"off"`
}
