package power_test

import (
	"testing"

	"github.com/cristian/holocron/internal/library"
	"github.com/cristian/holocron/internal/power"
	"github.com/cristian/holocron/internal/torrents"
)

// The real services must satisfy the probe interfaces, or the wiring in main
// would be the first place to find out.
func TestRealServicesSatisfyTheProbes(t *testing.T) {
	var _ power.MediaProbe = (*library.Service)(nil)
	var _ power.TorrentProbe = (*torrents.Service)(nil)
}
