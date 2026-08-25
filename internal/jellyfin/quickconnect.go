package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// Quick Connect authorises this device by showing the user a short code to
// approve in Jellyfin, instead of asking them to paste an API key.

// ErrQuickConnectDisabled means the server has the feature switched off. It is
// opt-in in the Jellyfin dashboard, so this is worth reporting plainly rather
// than as a generic failure.
var ErrQuickConnectDisabled = errors.New("quick connect is disabled on this server")

// QuickConnectState is a pending authorisation.
type QuickConnectState struct {
	Authenticated bool   `json:"Authenticated"`
	Secret        string `json:"Secret"`
	Code          string `json:"Code"`
	DeviceID      string `json:"DeviceId"`
}

// QuickConnectEnabled reports whether the server accepts Quick Connect. This
// endpoint needs no authentication at all.
func (c *Client) QuickConnectEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	if err := c.do(ctx, http.MethodGet, "/QuickConnect/Enabled", &enabled); err != nil {
		return false, err
	}
	return enabled, nil
}

// InitiateQuickConnect asks for a code to show the user.
//
// This is a POST, and it requires the client identification header even though
// there is no token yet — without it the server answers 400 with a message
// that explains nothing. Sending the full header also makes Jellyfin adopt our
// device id instead of inventing one, which is what lets the session be
// recognised and revoked later.
func (c *Client) InitiateQuickConnect(ctx context.Context) (QuickConnectState, error) {
	enabled, err := c.QuickConnectEnabled(ctx)
	if err != nil {
		return QuickConnectState{}, err
	}
	if !enabled {
		return QuickConnectState{}, ErrQuickConnectDisabled
	}

	var out QuickConnectState
	if err := c.do(ctx, http.MethodPost, "/QuickConnect/Initiate", &out); err != nil {
		return QuickConnectState{}, err
	}
	return out, nil
}

// QuickConnectStatus reports whether the user has approved the code yet.
func (c *Client) QuickConnectStatus(ctx context.Context, secret string) (bool, error) {
	q := url.Values{"secret": {secret}}
	var out QuickConnectState
	if err := c.do(ctx, http.MethodGet, "/QuickConnect/Connect?"+q.Encode(), &out); err != nil {
		return false, err
	}
	return out.Authenticated, nil
}

// Authentication is the result of a completed Quick Connect.
type Authentication struct {
	Token  string
	UserID string
	User   string
	Admin  bool
}

type authResponse struct {
	AccessToken string `json:"AccessToken"`
	User        struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
		} `json:"Policy"`
	} `json:"User"`
}

// RedeemQuickConnect exchanges an approved secret for an access token.
//
// Admin is reported because asking Jellyfin to write metadata requires an
// administrator: linking with a regular account succeeds but leaves that action
// returning 403, and saying so up front beats failing later.
func (c *Client) RedeemQuickConnect(ctx context.Context, secret string) (Authentication, error) {
	q := url.Values{"secret": {secret}}
	var out authResponse
	if err := c.do(ctx, http.MethodPost,
		"/Users/AuthenticateWithQuickConnect?"+q.Encode(), &out); err != nil {
		return Authentication{}, err
	}
	if out.AccessToken == "" {
		return Authentication{}, errors.New("jellyfin returned no access token")
	}
	return Authentication{
		Token:  out.AccessToken,
		UserID: out.User.ID,
		User:   out.User.Name,
		Admin:  out.User.Policy.IsAdministrator,
	}, nil
}
