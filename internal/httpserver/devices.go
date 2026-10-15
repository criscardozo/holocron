package httpserver

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"path"

	"github.com/cristian/holocron/internal/devices"
	"github.com/cristian/holocron/web/templates"
)

// handleDevices is the house's devices: a card per device, named after a
// city, and the network's plumbing as a list at the end.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, templates.DevicesPage(s.devicesView()))
}

func (s *Server) devicesView() templates.DevicesView {
	v := templates.DevicesView{Configured: s.deps.Devices.Configured(), File: devices.FileName}
	if !v.Configured {
		return v
	}
	groups, err := s.deps.Devices.Load()
	if err != nil {
		var fe *devices.FileError
		switch {
		case errors.As(err, &fe):
			v.ErrorLine = fe.Line
		case errors.Is(err, fs.ErrNotExist):
			v.Missing = true
		default:
			v.Failed = true
		}
		s.log.Warn("read devices", "error", err)
		return v
	}
	for _, g := range groups {
		gv := templates.DeviceGroup{Name: g.Name, Plain: g.Plain}
		for _, d := range g.Devices {
			gv.Devices = append(gv.Devices, templates.DeviceCard{
				Name: d.Name, Model: d.Model, IP: d.IP, MAC: d.MAC,
				CityURL: photoURL(d.CityPhoto), DeviceURL: photoURL(d.DevicePhoto),
			})
		}
		v.Groups = append(v.Groups, gv)
	}
	return v
}

func photoURL(name string) string {
	if name == "" {
		return ""
	}
	return "/devices/photo/" + url.PathEscape(name)
}

// handleDevicePhoto serves a photo from the devices directory. The browser
// never fetches them from the internet: they were downloaded once and live
// on the server.
func (s *Server) handleDevicePhoto(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.deps.Devices.ReadPhoto(name)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, devices.ErrNotConfigured) {
			s.log.Warn("read device photo", "error", err)
		}
		http.NotFound(w, r)
		return
	}
	// A day: the photos change when someone swaps one by hand, and
	// Last-Modified lets the browser check cheaply after that.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, path.Base(name), p.ModTime, bytes.NewReader(p.Data))
}
