package server

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/dan/moe/internal/models"
)

// ── Template data ───────────────────────────────────────────────────────

type deviceListData struct {
	Nav       string
	Devices   []models.Device
	Filter    models.DeviceFilter
	Total     int
	Providers []string
	OSList    []string
}

type deviceFormData struct {
	Nav       string
	Device    *models.Device
	Providers []models.ProviderConfig
	IsNew     bool
	Error     string
}

// ── Handlers ────────────────────────────────────────────────────────────

func (s *Server) handleDeviceList(w http.ResponseWriter, r *http.Request) {
	filter := models.DeviceFilter{
		ProviderName: r.URL.Query().Get("provider"),
		OS:           r.URL.Query().Get("os"),
		Compliance:   r.URL.Query().Get("compliance"),
		Search:       r.URL.Query().Get("q"),
		Limit:        500,
		Offset:       0,
	}

	devices, total, err := s.devices.List(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	providers, _ := s.devices.DistinctProviders()
	osList, _ := s.devices.DistinctOS()

	s.render.render(w, "devices.html", deviceListData{
		Nav:       "devices",
		Devices:   devices,
		Filter:    filter,
		Total:     total,
		Providers: providers,
		OSList:    osList,
	})
}

// handleDeviceRows renders just the table rows for htmx partial updates.
func (s *Server) handleDeviceRows(w http.ResponseWriter, r *http.Request) {
	filter := models.DeviceFilter{
		ProviderName: r.URL.Query().Get("provider"),
		OS:           r.URL.Query().Get("os"),
		Compliance:   r.URL.Query().Get("compliance"),
		Search:       r.URL.Query().Get("q"),
		Limit:        500,
		Offset:       0,
	}

	devices, _, err := s.devices.List(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.render.renderBlock(w, "devices.html", "device-rows", struct {
		Devices []models.Device
	}{
		Devices: devices,
	})
}

func (s *Server) handleDeviceNew(w http.ResponseWriter, r *http.Request) {
	providers, _ := s.providerConfigs.ListAll()

	s.render.render(w, "device_form.html", deviceFormData{
		Nav:       "devices",
		Device:    &models.Device{Compliance: "unknown"},
		Providers: providers,
		IsNew:     true,
	})
}

func (s *Server) handleDeviceCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	providers, _ := s.providerConfigs.ListAll()

	d := &models.Device{
		ID:           newID(),
		ProviderName: r.FormValue("provider_name"),
		SourceID:     r.FormValue("source_id"),
		DeviceName:   r.FormValue("device_name"),
		OS:           r.FormValue("os"),
		OSVersion:    r.FormValue("os_version"),
		Model:        r.FormValue("model"),
		UserName:     r.FormValue("user_name"),
		UserEmail:    r.FormValue("user_email"),
		Compliance:   r.FormValue("compliance"),
	}

	// Look up provider type from config.
	for _, p := range providers {
		if p.Name == d.ProviderName {
			d.ProviderType = p.Type
			break
		}
	}
	if d.ProviderType == "" {
		d.ProviderType = "uem" // fallback
	}

	if d.ProviderName == "" || d.DeviceName == "" {
		s.render.render(w, "device_form.html", deviceFormData{
			Nav:       "devices",
			Device:    d,
			Providers: providers,
			IsNew:     true,
			Error:     "Provider and device name are required.",
		})
		return
	}

	if err := s.devices.Create(d); err != nil {
		s.render.render(w, "device_form.html", deviceFormData{
			Nav:       "devices",
			Device:    d,
			Providers: providers,
			IsNew:     true,
			Error:     err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/devices?flash=Device+created&flash_type=success", http.StatusSeeOther)
}

func (s *Server) handleDeviceEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.devices.GetByID(id)
	if err != nil || d == nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	providers, _ := s.providerConfigs.ListAll()

	s.render.render(w, "device_form.html", deviceFormData{
		Nav:       "devices",
		Device:    d,
		Providers: providers,
		IsNew:     false,
	})
}

func (s *Server) handleDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.devices.GetByID(id)
	if err != nil || d == nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	providers, _ := s.providerConfigs.ListAll()

	d.ProviderName = r.FormValue("provider_name")
	d.SourceID = r.FormValue("source_id")
	d.DeviceName = r.FormValue("device_name")
	d.OS = r.FormValue("os")
	d.OSVersion = r.FormValue("os_version")
	d.Model = r.FormValue("model")
	d.UserName = r.FormValue("user_name")
	d.UserEmail = r.FormValue("user_email")
	d.Compliance = r.FormValue("compliance")

	for _, p := range providers {
		if p.Name == d.ProviderName {
			d.ProviderType = p.Type
			break
		}
	}

	if d.ProviderName == "" || d.DeviceName == "" {
		s.render.render(w, "device_form.html", deviceFormData{
			Nav:       "devices",
			Device:    d,
			Providers: providers,
			IsNew:     false,
			Error:     "Provider and device name are required.",
		})
		return
	}

	if err := s.devices.Update(d); err != nil {
		s.render.render(w, "device_form.html", deviceFormData{
			Nav:       "devices",
			Device:    d,
			Providers: providers,
			IsNew:     false,
			Error:     err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/devices?flash=Device+updated&flash_type=success", http.StatusSeeOther)
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.devices.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices?flash=Device+deleted&flash_type=success", http.StatusSeeOther)
}

// newID generates a short random hex ID.
func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
