package jellyfin

import (
	"context"
	"errors"
	"strconv"

	"github.com/cristian/holocron/internal/settings"
)

// ErrNotLinked means no Jellyfin address and token have been stored yet, so
// there is nothing to talk to. Shared by every caller that builds a client from
// settings, so they all agree on what "configured" means.
var ErrNotLinked = errors.New("jellyfin is not linked")

// Linked reports whether both the address and a token are stored.
func Linked(ctx context.Context, st *settings.Store) bool {
	return st.GetDefault(ctx, settings.KeyJellyfinURL, "") != "" &&
		st.GetDefault(ctx, settings.KeyJellyfinToken, "") != ""
}

// IsAdmin reports whether the linked account is a Jellyfin administrator.
// Asking the server to re-read metadata requires one, so the UI has to know
// before offering the button rather than after the 403.
func IsAdmin(ctx context.Context, st *settings.Store) bool {
	// Stored with strconv.FormatBool when the link is redeemed.
	admin, err := strconv.ParseBool(st.GetDefault(ctx, settings.KeyJellyfinAdmin, ""))
	return err == nil && admin
}

// FromSettings builds a client from the stored address and token. version is
// reported to Jellyfin, which lists it beside the device.
func FromSettings(ctx context.Context, st *settings.Store, version string) (*Client, error) {
	base := st.GetDefault(ctx, settings.KeyJellyfinURL, "")
	token := st.GetDefault(ctx, settings.KeyJellyfinToken, "")
	if base == "" || token == "" {
		return nil, ErrNotLinked
	}
	device := st.GetDefault(ctx, settings.KeyJellyfinDeviceID, "")
	return New(base, token, device, version), nil
}
