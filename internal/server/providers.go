package server

import (
	"fmt"
	"net/http"

	"github.com/dan/moe/internal/models"
)

// ── Template data ───────────────────────────────────────────────────────

type providerListData struct {
	Nav          string
	Providers    []models.ProviderConfig
	DeviceCounts map[string]int
	Statuses     map[string]*ProviderStatus
}

type providerFormData struct {
	Nav      string
	Provider *models.ProviderConfig
	IsNew    bool
	Error    string
}

// ── Handlers ────────────────────────────────────────────────────────────

func (s *Server) handleProviderList(w http.ResponseWriter, r *http.Request) {
	providers, err := s.providerConfigs.ListAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	deviceCounts, _ := s.devices.CountByProvider()

	s.render.render(w, "providers.html", providerListData{
		Nav:          "providers",
		Providers:    providers,
		DeviceCounts: deviceCounts,
		Statuses:     s.status.All(),
	})
}

func (s *Server) handleProviderNew(w http.ResponseWriter, r *http.Request) {
	s.render.render(w, "provider_form.html", providerFormData{
		Nav:      "providers",
		Provider: &models.ProviderConfig{SyncInterval: "15m", Enabled: true},
		IsNew:    true,
	})
}

func (s *Server) handleProviderCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p := &models.ProviderConfig{
		ID:           newID(),
		Name:         r.FormValue("name"),
		Type:         r.FormValue("type"),
		SyncInterval: r.FormValue("sync_interval"),
		Enabled:      r.FormValue("enabled") == "on",
	}

	// Populate type-specific fields.
	switch p.Type {
	case "intune":
		p.TenantID = r.FormValue("tenant_id")
		p.ClientID = r.FormValue("client_id")
		p.ClientSecret = r.FormValue("client_secret")
	case "uem":
		p.BaseURL = r.FormValue("base_url")
		p.TenantID = r.FormValue("uem_tenant_id")
		p.Username = r.FormValue("username")
		p.Password = r.FormValue("password")
	}

	if p.Name == "" || p.Type == "" {
		s.render.render(w, "provider_form.html", providerFormData{
			Nav:      "providers",
			Provider: p,
			IsNew:    true,
			Error:    "Name and type are required.",
		})
		return
	}

	if err := s.providerConfigs.Create(p); err != nil {
		s.render.render(w, "provider_form.html", providerFormData{
			Nav:      "providers",
			Provider: p,
			IsNew:    true,
			Error:    err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/providers?flash=Provider+"+p.Name+"+created&flash_type=success", http.StatusSeeOther)
}

func (s *Server) handleProviderEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.providerConfigs.GetByID(id)
	if err != nil || p == nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	s.render.render(w, "provider_form.html", providerFormData{
		Nav:      "providers",
		Provider: p,
		IsNew:    false,
	})
}

func (s *Server) handleProviderUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.providerConfigs.GetByID(id)
	if err != nil || p == nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p.Name = r.FormValue("name")
	p.Type = r.FormValue("type")
	p.SyncInterval = r.FormValue("sync_interval")
	p.Enabled = r.FormValue("enabled") == "on"

	// Populate type-specific fields; clear the other type's fields.
	switch p.Type {
	case "intune":
		p.TenantID = r.FormValue("tenant_id")
		p.ClientID = r.FormValue("client_id")
		if secret := r.FormValue("client_secret"); secret != "" {
			p.ClientSecret = secret
		}
		// Clear UEM fields.
		p.BaseURL = ""
		p.Username = ""
		p.Password = ""
	case "uem":
		p.BaseURL = r.FormValue("base_url")
		p.TenantID = r.FormValue("uem_tenant_id")
		p.Username = r.FormValue("username")
		if pw := r.FormValue("password"); pw != "" {
			p.Password = pw
		}
		// Clear Intune fields.
		p.ClientID = ""
		p.ClientSecret = ""
	}

	if p.Name == "" || p.Type == "" {
		s.render.render(w, "provider_form.html", providerFormData{
			Nav:      "providers",
			Provider: p,
			IsNew:    false,
			Error:    "Name and type are required.",
		})
		return
	}

	if err := s.providerConfigs.Update(p); err != nil {
		s.render.render(w, "provider_form.html", providerFormData{
			Nav:      "providers",
			Provider: p,
			IsNew:    false,
			Error:    err.Error(),
		})
		return
	}

	http.Redirect(w, r, "/providers?flash=Provider+"+p.Name+"+updated&flash_type=success", http.StatusSeeOther)
}

func (s *Server) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.providerConfigs.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/providers?flash=Provider+deleted&flash_type=success", http.StatusSeeOther)
}

// handleProviderToggle enables or disables a provider. POST /providers/{id}/toggle
func (s *Server) handleProviderToggle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg, err := s.providerConfigs.GetByID(id)
	if err != nil || cfg == nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	newState := !cfg.Enabled
	if err := s.providerConfigs.SetEnabled(id, newState); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	action := "disabled"
	flashType := "success"
	if newState {
		action = "enabled"
		// Trigger an immediate health check on re-enable.
		go s.CheckProviderNow(cfg.Name, cfg.Type)
	} else {
		// Clear in-memory status when disabled.
		s.status.Remove(cfg.Name)
	}

	s.activity.Logf(cfg.Name, "info", "Provider %s by operator", action)
	http.Redirect(w, r, fmt.Sprintf("/providers?flash=%s+%s&flash_type=%s", cfg.Name, action, flashType), http.StatusSeeOther)
}
