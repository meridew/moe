package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dan/moe/internal/models"
	"github.com/dan/moe/internal/provider"
	"github.com/dan/moe/internal/provider/intune"
)

// handleProviderSync triggers an immediate device sync for a provider.
// POST /providers/{id}/sync
func (s *Server) handleProviderSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg, err := s.providerConfigs.GetByID(id)
	if err != nil || cfg == nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	p, err := s.buildProvider(cfg)
	if err != nil {
		s.activity.Logf(cfg.Name, "error", "Sync failed — could not initialise provider: %s", err)
		http.Error(w, "Failed to initialise provider: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.activity.Logf(cfg.Name, "info", "Sync started…")
	count, syncErr := s.syncProvider(r.Context(), p)
	if syncErr != nil {
		log.Printf("[sync] error syncing %s: %v", cfg.Name, syncErr)
		s.activity.Logf(cfg.Name, "error", "Sync failed: %s", syncErr)
		http.Redirect(w, r, fmt.Sprintf("/providers?flash=%s: %s&flash_type=error", cfg.Name, syncErr.Error()), http.StatusSeeOther)
		return
	}

	log.Printf("[sync] completed %s: %d devices synced", cfg.Name, count)
	s.activity.Logf(cfg.Name, "success", "Sync complete — %d devices", count)
	_ = s.providerConfigs.RecordSyncSuccess(cfg.Name)
	http.Redirect(w, r, fmt.Sprintf("/providers?flash=Synced %s — %d devices&flash_type=success", cfg.Name, count), http.StatusSeeOther)
}

// buildProvider creates a Provider instance from a ProviderConfig.
func (s *Server) buildProvider(cfg *models.ProviderConfig) (provider.Provider, error) {
	switch cfg.Type {
	case "intune":
		return intune.New(intune.Config{
			Name:         cfg.Name,
			TenantID:     cfg.TenantID,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
		}), nil
	case "uem":
		return nil, fmt.Errorf("UEM provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// syncProvider runs a full device sync for the given provider, upserting all
// returned devices into the local cache. Returns the total device count.
func (s *Server) syncProvider(ctx context.Context, p provider.Provider) (int, error) {
	var (
		cursor string
		total  int
	)

	for {
		devices, nextCursor, err := p.SyncDevices(ctx, cursor)
		if err != nil {
			return total, fmt.Errorf("sync page: %w", err)
		}

		now := time.Now().UTC()
		for _, sd := range devices {
			d := &models.Device{
				ID:           newID(),
				ProviderName: p.Name(),
				ProviderType: p.Type(),
				SourceID:     sd.SourceID,
				DeviceName:   sd.DeviceName,
				OS:           sd.OS,
				OSVersion:    sd.OSVersion,
				Model:        sd.Model,
				UserName:     sd.UserName,
				UserEmail:    sd.UserEmail,
				Compliance:   sd.Compliance,
				LastSeen:     sd.LastSeen,
				LastSyncedAt: &now,
				CreatedAt:    now,
			}
			if err := s.devices.Upsert(d); err != nil {
				log.Printf("[sync] upsert error for %s/%s: %v", p.Name(), sd.SourceID, err)
			}
		}

		total += len(devices)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return total, nil
}
