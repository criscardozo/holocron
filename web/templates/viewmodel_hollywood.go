package templates

// HollywoodView drives the Hollywood page: the TV launcher's settings.
type HollywoodView struct {
	// URL is the launcher's settings page on the domain the page was asked
	// for.
	URL string
	// Checked says whether the server knows where the launcher is; Up,
	// whether it answered just now.
	Checked bool
	Up      bool
}
